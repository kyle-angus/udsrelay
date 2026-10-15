// Command udstrace turns Azahar logs captured with recon/azahar/udstrace.patch into a merged
// UDS command timeline, a Markdown command-trace summary, or JSON lines.
//
//	udstrace timeline [-v] host=host.log client=client.log
//	udstrace summary [-title "Pokémon X/Y trade"] [-o trace.md] host=host.log client=client.log
//	udstrace compare azahar=recon/traces/trade-01/host.log console=shim-host.log
//	udstrace json host=host.log
//	udstrace redact console-host.log recon/traces/battle-hw-01/console-host.log
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"udsrelay/internal/udstrace"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: udstrace <timeline|summary|compare|json> [flags] [label=]log ...
       udstrace redact in.log out.log

Each log is one instance's trace: an Azahar log or a console shim log. The label (default:
file name) names the instance in the output, e.g. host=host/user/log/azahar_log.txt.

compare takes exactly two logs for the same role, reference first:
  udstrace compare azahar=azahar-host.log console=console-host.log
It exits 1 when the traces differ.

redact copies a console log with identifying data removed: beacon payloads emptied (the
game's first scan returns all nearby Wi-Fi access points), the host MAC in netinfo zeroed,
and NodeInfo friend-code seeds zeroed, also inside data frames (X/Y sends them during a
trade). Changed lines are marked redacted=<what>. Redact
hardware logs before committing them.

flags:
  -v              timeline: show every poll, data packet, payload preview and debug note
  -title s        summary: report title (default: the labels)
  -shift label=ms move one instance's timestamps by ms milliseconds (repeatable); console
                  clocks are not synchronised
  -o file         write output to file instead of stdout
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	mode := os.Args[1]
	if mode == "redact" {
		redact(os.Args[2:])
		return
	}
	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	fs.Usage = usage
	verbose := fs.Bool("v", false, "")
	title := fs.String("title", "", "")
	outPath := fs.String("o", "", "")
	shifts := map[string]int64{}
	fs.Func("shift", "", func(v string) error {
		label, ms, ok := strings.Cut(v, "=")
		if !ok {
			return fmt.Errorf("want label=milliseconds, got %q", v)
		}
		n, err := strconv.ParseFloat(ms, 64)
		if err != nil {
			return fmt.Errorf("shift %s: %w", label, err)
		}
		shifts[label] = int64(n * 1000)
		return nil
	})
	if err := fs.Parse(os.Args[2:]); err != nil || fs.NArg() == 0 {
		usage()
	}

	sessions, err := load(fs.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "udstrace:", err)
		os.Exit(1)
	}
	for _, s := range sessions {
		if us, ok := shifts[s.Instance]; ok {
			s.ShiftWall(us)
			delete(shifts, s.Instance)
		}
	}
	for label := range shifts {
		fmt.Fprintf(os.Stderr, "udstrace: -shift names unknown instance %q\n", label)
		os.Exit(2)
	}

	var out io.Writer = os.Stdout
	if *outPath != "" {
		f, err := os.Create(*outPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "udstrace:", err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}

	exitCode := 0
	switch mode {
	case "timeline":
		udstrace.WriteTimeline(out, sessions, *verbose)
	case "summary":
		t := *title
		if t == "" {
			labels := make([]string, len(sessions))
			for i, s := range sessions {
				labels[i] = s.Instance
			}
			t = strings.Join(labels, " + ")
		}
		udstrace.WriteSummary(out, sessions, t)
	case "compare":
		if len(sessions) != 2 {
			fmt.Fprintln(os.Stderr, "udstrace: compare takes exactly two logs: reference, then candidate")
			os.Exit(2)
		}
		if !udstrace.WriteCompare(out, sessions[0], sessions[1]).Matches() {
			exitCode = 1
		}
	case "json":
		enc := json.NewEncoder(out)
		for _, s := range sessions {
			for _, r := range s.Records {
				if err := enc.Encode(r); err != nil {
					fmt.Fprintln(os.Stderr, "udstrace:", err)
					os.Exit(1)
				}
			}
		}
	default:
		usage()
	}
	if exitCode != 0 {
		if f, ok := out.(*os.File); ok && f != os.Stdout {
			f.Close()
		}
		os.Exit(exitCode)
	}
}

// redact copies a log with identifying data removed: udstrace redact in.log out.log
func redact(args []string) {
	if len(args) != 2 || args[0] == args[1] {
		usage()
	}
	in, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "udstrace:", err)
		os.Exit(1)
	}
	defer in.Close()
	out, err := os.Create(args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "udstrace:", err)
		os.Exit(1)
	}
	n, err := udstrace.Redact(in, out)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "udstrace:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "redacted %d line(s) into %s\n", n, args[1])
}

func load(args []string) ([]*udstrace.Session, error) {
	var sessions []*udstrace.Session
	traced := 0
	for _, arg := range args {
		label, path, ok := strings.Cut(arg, "=")
		if !ok {
			path = arg
			label = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		recs, err := udstrace.Parse(f, label)
		f.Close()
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			if r.Kind != "note" {
				traced++
				break
			}
		}
		sessions = append(sessions, udstrace.NewSession(label, recs))
	}
	if traced == 0 {
		return nil, fmt.Errorf("no UDSTRACE lines found; was Azahar built with recon/azahar/udstrace.patch?")
	}
	return sessions, nil
}

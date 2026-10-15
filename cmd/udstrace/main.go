// Command udstrace turns Azahar logs captured with recon/azahar/udstrace.patch into a merged
// UDS command timeline, a Markdown command-trace summary, or JSON lines.
//
//	udstrace timeline [-v] host=host.log client=client.log
//	udstrace summary [-title "Pokémon X/Y trade"] [-o trace.md] host=host.log client=client.log
//	udstrace json host=host.log
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"udsrelay/internal/udstrace"
)

func usage() {
	fmt.Fprint(os.Stderr, `usage: udstrace <timeline|summary|json> [flags] [label=]log ...

Each log is an Azahar log from one instance. The label (default: file name) names the
instance in the output, e.g. host=host/user/log/azahar_log.txt.

flags:
  -v        timeline: show every poll, data packet, payload preview and debug note
  -title s  summary: report title (default: the labels)
  -o file   write output to file instead of stdout
`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	mode := os.Args[1]
	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	fs.Usage = usage
	verbose := fs.Bool("v", false, "")
	title := fs.String("title", "", "")
	outPath := fs.String("o", "", "")
	if err := fs.Parse(os.Args[2:]); err != nil || fs.NArg() == 0 {
		usage()
	}

	sessions, err := load(fs.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "udstrace:", err)
		os.Exit(1)
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

#include <3ds.h>
#include <CTRPluginFramework.hpp>

#include "shim_plugin.hpp"

namespace CTRPluginFramework {

// Runs before the game's first instruction, so the GetServiceHandle hook is in place before
// the game opens any service.
void PatchProcess(FwkSettings &settings) {
    (void)settings;
    udsrelay::Start();
}

int main(void) {
    PluginMenu *menu = new PluginMenu("UDS Relay shim", 0, 2, 0,
                                      "Phase 2 passthrough: forwards nwm::UDS and ndm:u to the real "
                                      "services and logs every call to sdmc:/udsrelay/logs.");
    menu->SynchronizeWithFrame(true);
    *menu += new MenuEntry("Status", nullptr, [](MenuEntry *) { MessageBox("UDS Relay shim", udsrelay::StatusText())(); },
                           "Hooks, intercepted sessions and log file.");
    menu->Run();
    delete menu;
    return 0;
}

} // namespace CTRPluginFramework

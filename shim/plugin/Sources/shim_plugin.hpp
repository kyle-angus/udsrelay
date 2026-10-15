#pragma once

#include <string>

namespace udsrelay {

constexpr const char *kShimVersion = "0.2.0-passthrough";

/// Loads config, opens the SD log, hooks srv:GetServiceHandle. Call from PatchProcess.
void Start();

/// Multi-line status for the plugin menu.
std::string StatusText();

} // namespace udsrelay

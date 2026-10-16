#pragma once

#include <string>

namespace udsrelay {

constexpr const char *kShimVersion = "0.3.3";

/// Loads config, opens the SD log, hooks srv:GetServiceHandle. Call from PatchProcess.
void Start();

/// Multi-line status for the plugin menu.
std::string StatusText();

} // namespace udsrelay

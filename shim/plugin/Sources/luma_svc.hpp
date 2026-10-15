// The one Luma3DS custom syscall the shim uses. Neither libctru nor libctrpf declares it, and
// Luma's full csvc.h is not installed by devkitPro, so this declares just what shim.cpp needs.
#pragma once

#include <3ds/types.h>

namespace udsrelay {

// Luma3DS ServiceOp values for svcControlService (SVC 0xB0).
enum LumaServiceOp : u32 {
    SERVICEOP_STEAL_CLIENT_SESSION = 0,
    SERVICEOP_GET_NAME = 1,  // name of the service or port behind a session handle, into a 12-byte buffer
};

// Arguments arrive in r0-r2 as the syscall expects them, so the body is just the SVC.
__attribute__((naked)) static Result svcControlService(LumaServiceOp, char *, Handle) {
    __asm__ volatile("svc 0xB0\n\tbx lr");
}

}  // namespace udsrelay

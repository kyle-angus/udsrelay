@ Luma3DS custom system call used by the sysmodule, from Luma3DS's
@ sysmodules/rosalina/source/csvc.s (Copyright (C) 2016-2020 Aurora Wright, TuxSH;
@ SPDX-License-Identifier: MIT OR GPL-2.0-or-later).
@
@ Result svcControlMemoryEx(u32 *addr_out, u32 addr0, u32 addr1, u32 size, MemOp op, MemPerm perm, bool isLoader);

.arm
.balign 4

.section .text.svcControlMemoryEx, "ax", %progbits
.global svcControlMemoryEx
.type svcControlMemoryEx, %function
.align 2
svcControlMemoryEx:
    push {r0, r4, r5}
    ldr  r0, [sp, #0xC]
    ldr  r4, [sp, #0xC+0x4]
    ldr  r5, [sp, #0xC+0x8]
    svc  0xA2
    pop  {r2, r4, r5}
    str  r1, [r2]
    bx   lr

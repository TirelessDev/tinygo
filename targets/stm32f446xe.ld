
MEMORY
{
    FLASH_TEXT (rw) : ORIGIN = 0x8000000, LENGTH = 512K
    RAM (xrw)       : ORIGIN = 0x20000000, LENGTH = 128K
}

_stack_size = 4K;

INCLUDE "targets/arm.ld"

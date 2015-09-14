#define SYS_BASE 0x0
#define SYS_clock_gettime (SYS_BASE + 263)

TEXT ·monotime(SB), 7, $32
	MOVW	$4, R0  // CLOCK_MONOTONIC_RAW
	MOVW	$8(R13), R1
	MOVW	$SYS_clock_gettime, R7
	SWI	$0

	MOVW	8(R13), R0
	MOVW	12(R13), R2

	MOVW	R0, 0(FP)
	MOVW	$0, R1
	MOVW	R1, 4(FP)
	MOVW	R2, 8(FP)
	RET

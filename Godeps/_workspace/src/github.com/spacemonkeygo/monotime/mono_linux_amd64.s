TEXT ·monotime(SB),7,$16
	MOVQ	runtime·__vdso_clock_gettime_sym(SB), AX
	CMPQ	AX, $0
	JEQ	vdso_is_sad
	MOVL	$4, DI  // CLOCK_MONOTONIC_RAW
	LEAQ	0(SP), SI
	CALL	AX
	MOVQ	0(SP), AX
	MOVQ	8(SP), DX
	MOVQ	AX, sec+0(FP)
	MOVL	DX, nsec+8(FP)
	RET
vdso_is_sad:
	MOVQ	$0, sec+0(FP)
	RET

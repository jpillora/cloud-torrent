.PHONY: gen-types check-types

# Regenerate the frontend TypeScript types from the Go structs (engine + server).
gen-types:
	go tool tygo generate

# CI guard: regenerate and fail if the committed types are out of date.
check-types: gen-types
	git diff --exit-code -- frontend/src/types

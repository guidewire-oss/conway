# Conway's task interface lives in Makefile.conway; this file only includes it.
#
# Why the split: the software factory's installer replaces Makefile wholesale
# with its own governance targets (selftest, doctor, check, sync-harnesses,
# pre-push). Keeping Conway's targets in a separate file means recovering from
# that costs one `include` line rather than the whole interface. If you find this
# file overwritten and `make build` gone, add the include back.
#
# `make` with no target runs the first target in the included file: help.

include Makefile.conway

# proto-gen
This package implements a basic protobuf compiler for pbs (streaming protobufs).
It is not yet complete, but works for basic protobuf files.

## Currently not handled:
- enums
- options
- default values
- comments
- using other top level messages inside eachother
	- currently supports using sub-message
- importing
- groups
- probably other things

There is no reason why these are not implemented other than the fact that
I havent needed them yet, so I havent written them.


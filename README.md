# go-pbs
An implementation of streaming protocol buffers in go.

See [mafintosh's javascript impl](http://github.com/mafintosh/pbs) for more info

This implementation is still very WIP and does not fully implement the protobuf spec.

You will also need to manually modify your \*.pb.go files to make them implement the 
`StreamMessage` interface. All repeated fields will need to be changed to channels.
I plan on writing a code generator soon to do this from .proto files.

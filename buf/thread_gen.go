package buf

//go:generate buf generate ../../protos/im --template ./buf.gen.thread.yaml --path ../../protos/im/internal/thread/v1

//go:generate buf generate ../../protos/im --template ./buf.gen.thread.yaml --path ../../protos/im/shared/thread/v1

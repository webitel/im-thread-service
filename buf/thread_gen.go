package buf

//go:generate buf generate ../../protos/im --template ./buf.gen.thread.yaml --path ../../protos/im/service/thread/v1

//go:generate buf generate ../../protos/im --template ./buf.gen.thread.yaml --path ../../protos/im/domain/thread/v1

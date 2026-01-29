BUF_DIR := buf
THREAD_PROTO_INTERNAL := ../../protos/im/service/thread/v1
THREAD_PROTO_SHARED := ../../protos/im/domain/thread/v1

.PHONY: gen-thread

gen-thread:
	@echo "Generating thread protos"
	cd $(BUF_DIR)/ && go run github.com/bufbuild/buf/cmd/buf@latest generate \
		--template buf.gen.thread.yaml \
		--path $(THREAD_PROTO_INTERNAL) \
		--path $(THREAD_PROTO_SHARED)
	@echo "End of generating thread protos."
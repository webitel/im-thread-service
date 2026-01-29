package buf

// Generate Contact base API
//go:generate buf generate ../../protos/im --template ./buf.gen.contact.yaml --path ../../protos/im/service/contact/v1

// Generate Contact shared components
//go:generate buf generate ../../protos/im --template ./buf.gen.contact.yaml --path ../../protos/im/domain/contact/v1

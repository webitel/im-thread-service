package model

import "time"

const (
	ServiceName      = "im-contact-service"
	ServiceNamespace = "webitel"
	Version          = "1.0.0"
)

var (
	Commit         = "hash"
	CommitDate     = time.Now().String()
	Branch         = "branch"
	BuildTimestamp = ""
)

package utils

import (
	"net/url"

	"github.com/webitel/webitel-go-kit/pkg/errors"
)

func ResolveFullURL(baseStr, relativeStr string) (string, error) {
	if baseStr == "" {
		return "", errors.InvalidArgument("base URL cannot be empty", errors.WithID("utils.url.resolve_full_url"))
	}

	if relativeStr == "" {
		return "", errors.InvalidArgument("relative URL cannot be empty", errors.WithID("utils.url.resolve_full_url"))
	}

	base, err := url.Parse(baseStr)
	if err != nil {
		return "", errors.InvalidArgument("parsing base URL", errors.WithCause(err), errors.WithID("utils.url.resolve_full_url"))
	}

	rel, err := url.Parse(relativeStr)
	if err != nil {
		return "", errors.InvalidArgument("parsing relative URL", errors.WithCause(err), errors.WithID("utils.url.resolve_full_url"))
	}

	return base.ResolveReference(rel).String(), nil
}

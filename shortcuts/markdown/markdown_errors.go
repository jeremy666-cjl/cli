// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"strings"

	"github.com/larksuite/cli/errs"
)

func markdownValidationError(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func markdownValidationParamError(param, format string, args ...any) *errs.ValidationError {
	return markdownValidationError(format, args...).WithParam(param)
}

func markdownInvalidParam(name, reason string) errs.InvalidParam {
	return errs.InvalidParam{Name: name, Reason: reason}
}

func markdownNetworkError(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

func wrapMarkdownDownloadError(err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		if p.Category == errs.CategoryValidation {
			return err
		}
		return markdownPrefixProblem(err, "download failed")
	}
	return markdownNetworkError(err, "download failed: %s", err)
}

func markdownPrefixProblem(err error, action string) error {
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(action) != "" {
			p.Message = action + ": " + p.Message
		}
		return err
	}
	return errs.WrapInternal(err)
}

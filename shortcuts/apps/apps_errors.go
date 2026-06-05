// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
)

func appsValidationError(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func appsValidationParamError(param, format string, args ...any) *errs.ValidationError {
	return appsValidationError(format, args...).WithParam(param)
}

func appsInvalidParam(name, reason string) errs.InvalidParam {
	return errs.InvalidParam{Name: name, Reason: reason}
}

func appsFailedPreconditionParamError(param, format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, format, args...).WithParam(param)
}

func appsInputPathError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return appsValidationParamError("--path", "unsafe --path: %s", err).WithCause(err)
	}
	return appsValidationParamError("--path", "cannot read --path: %s", err).WithCause(err)
}

func appsInputPathEntryError(path string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return appsValidationParamError("--path", "unsafe --path entry %s: %s", path, err).WithCause(err)
	}
	return appsValidationParamError("--path", "cannot read --path entry %s: %s", path, err).WithCause(err)
}

func appsFileIOError(err error, format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeFileIO, format, args...).WithCause(err)
}

// enrichHTMLPublishAPIError adapts a typed failure from the HTML publish
// endpoint: refines endpoint-scoped business codes, prefixes the message with
// command context, and attaches endpoint-specific recovery hints. A
// still-untyped error is lifted at the SDK boundary instead.
func enrichHTMLPublishAPIError(err error) error {
	if err == nil {
		return nil
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		return client.WrapDoAPIError(err)
	}
	// The HTML publish business codes (90001/90002) are scoped to this
	// endpoint, not service-global, so their subtype classification lives
	// here instead of the global errclass code table. Only an
	// otherwise-unclassified API error is refined; a stronger upstream
	// classification is never overridden.
	if p.Category == errs.CategoryAPI && p.Subtype == errs.SubtypeUnknown && p.Code == errCodeAppNotFound {
		p.Subtype = errs.SubtypeNotFound
	}
	if p.Message != "" {
		p.Message = "html-publish failed: " + p.Message
	}
	if hint := buildHTMLPublishFailureHint(p.Code); hint != "" {
		p.Hint = hint
	}
	return err
}

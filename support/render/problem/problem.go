package problem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/support/log"
)

const horizonHost = "https://stellar.org/horizon-errors/"

// P is a struct that represents an error response to be rendered to a connected
// client.
type P struct {
	Type     string                 `json:"type"`
	Title    string                 `json:"title"`
	Status   int                    `json:"status"`
	Detail   string                 `json:"detail,omitempty"`
	Instance string                 `json:"instance,omitempty"`
	Extras   map[string]interface{} `json:"extras,omitempty"`
}

func (p P) Error() string {
	return fmt.Sprintf("problem: %s", p.Type)
}

var errToProblemMap = map[error]P{}

// RegisterError records an error -> P mapping, allowing the app to register
// specific errors that may occur in other packages to be rendered as a specific
// P instance.
//
// For example, you might want to render any sql.ErrNoRows errors as a
// problem.NotFound, and you would do so by calling:
//
// problem.RegisterError(sql.ErrNoRows, problem.NotFound) in you application
// initialization sequence
func RegisterError(err error, p P) {
	errToProblemMap[err] = p
}

// Inflate sets some basic parameters on the problem, mostly the type for now
func Inflate(p *P) {
	//TODO: add requesting url to extra info

	if !strings.HasPrefix(p.Type, horizonHost) {
		//TODO: make the horizonHost prefix configurable
		p.Type = horizonHost + p.Type
	}

	p.Instance = ""
}

// Render writes a http response to `w`, compliant with the "Problem
// Details for HTTP APIs" RFC:
// https://tools.ietf.org/html/draft-ietf-appsawg-http-problem-00
func Render(ctx context.Context, w http.ResponseWriter, err error) {
	origErr := errors.Cause(err)

	switch p := origErr.(type) {
	case P:
		renderProblem(ctx, w, p)
	case *P:
		renderProblem(ctx, w, *p)
	case error:
		renderErr(ctx, w, err)
	}
}

func renderProblem(ctx context.Context, w http.ResponseWriter, p P) {
	Inflate(&p)

	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")

	js, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		err = errors.Wrap(err, "failed to encode problem")
		log.Ctx(ctx).WithStack(err).Error(err)
		http.Error(w, "error rendering problem", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(p.Status)
	w.Write(js)
}

func renderErr(ctx context.Context, w http.ResponseWriter, err error) {
	origErr := errors.Cause(err)

	p, ok := errToProblemMap[origErr]

	// If this error is not a registered error
	// log it and replace it with a 500 error
	if !ok {
		log.Ctx(ctx).WithStack(err).Error(err)
		p = ServerError
	}

	renderProblem(ctx, w, p)
}

// ServerError is a well-known problem type. Use it as a shortcut.
var ServerError = P{
	Type:   "server_error",
	Title:  "Internal Server Error",
	Status: http.StatusInternalServerError,
	Detail: "An error occurred while processing this request.  This is usually due " +
		"to a bug within the server software.  Trying this request again may " +
		"succeed if the bug is transient, otherwise please report this issue " +
		"to the issue tracker at: https://github.com/stellar/go/issues." +
		" Please include this response in your issue.",
}

// NotFound is a well-known problem type.  Use it as a shortcut in your actions
var NotFound = P{
	Type:   "not_found",
	Title:  "Resource Missing",
	Status: http.StatusNotFound,
	Detail: "The resource at the url requested was not found.  This is usually " +
		"occurs for one of two reasons:  The url requested is not valid, or no " +
		"data in our database could be found with the parameters provided.",
}

// BadRequest is a well-known problem type.  Use it as a shortcut
// in your actions.
var BadRequest = P{
	Type:   "bad_request",
	Title:  "Bad Request",
	Status: http.StatusBadRequest,
	Detail: "The request you sent was invalid in some way",
}

// MakeInvalidFieldProblem is a helper function to make a BadRequest with extras
func MakeInvalidFieldProblem(name string, reason error) *P {
	br := BadRequest
	br.Extras = map[string]interface{}{
		"invalid_field": name,
		"reason":        reason.Error(),
	}
	return &br
}
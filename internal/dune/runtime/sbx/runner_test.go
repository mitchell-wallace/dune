package sbx

import (
	"context"
	"fmt"
)

type fakeRunner struct {
	calls     []fakeRunnerCall
	responses []fakeRunnerResponse
}

type fakeRunnerCall struct {
	stream bool
	dir    string
	name   string
	args   []string
}

type fakeRunnerResponse struct {
	output []byte
	err    error
}

func (r *fakeRunner) Capture(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	_ = ctx
	r.record(false, dir, name, args...)
	response, err := r.nextResponse()
	if err != nil {
		return nil, err
	}
	return response.output, response.err
}

func (r *fakeRunner) Stream(ctx context.Context, dir string, streams StdIO, name string, args ...string) error {
	_ = ctx
	_ = streams
	r.record(true, dir, name, args...)
	response, err := r.nextResponse()
	if err != nil {
		return err
	}
	return response.err
}

func (r *fakeRunner) record(stream bool, dir, name string, args ...string) {
	r.calls = append(r.calls, fakeRunnerCall{
		stream: stream,
		dir:    dir,
		name:   name,
		args:   append([]string(nil), args...),
	})
}

func (r *fakeRunner) nextResponse() (fakeRunnerResponse, error) {
	if len(r.responses) == 0 {
		return fakeRunnerResponse{}, fmt.Errorf("fakeRunner: no response configured for call %d", len(r.calls))
	}
	response := r.responses[0]
	r.responses = r.responses[1:]
	return response, nil
}

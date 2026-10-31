package streaming

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// host is an ordinary process/resource boundary, not a sandbox claim.
// Untrusted script text, results and proof bindings only cross private pipes.
type host struct {
	mu     sync.Mutex
	in     io.WriteCloser
	out    *bufio.Reader
	output io.Closer
	cancel context.CancelFunc
	done   chan struct{}
	id     uint64
	once   sync.Once
}

func startHost(command func(context.Context, string) *exec.Cmd, mode string) (*host, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := command(ctx, mode)
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, errors.New("runtime_missing")
	}
	out, writer, err := os.Pipe()
	if err != nil {
		in.Close()
		cancel()
		return nil, errors.New("runtime_missing")
	}
	cmd.Stdout = writer
	if cmd.Start() != nil {
		in.Close()
		out.Close()
		writer.Close()
		cancel()
		return nil, errors.New("runtime_missing")
	}
	writer.Close()
	h := &host{in: in, out: bufio.NewReader(out), output: out, cancel: cancel, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); in.Close(); close(h.done) }()
	return h, nil
}
func (h *host) Close() {
	if h != nil {
		h.once.Do(func() {
			h.in.Close()
			timer := time.NewTimer(4 * time.Second)
			select {
			case <-h.done:
				timer.Stop()
			case <-timer.C:
				h.cancel()
				<-h.done
			}
			h.cancel()
			h.output.Close()
		})
	}
}
func (h *host) call(ctx context.Context, code string, out any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	h.id++
	data, err := json.Marshal(struct {
		ID   uint64 `json:"id"`
		Code string `json:"code"`
	}{h.id, code})
	if err != nil || len(data) > 24<<20 {
		return errors.New("cache_limit")
	}
	stop := context.AfterFunc(ctx, h.Close)
	defer stop()
	data = append(data, '\n')
	if _, err = h.in.Write(data); err != nil {
		h.Close()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("script_process_failed")
	}
	// ReadSlice-style bounded framing avoids Scanner's token truncation and
	// never accepts an unbounded line before checking its size.
	var frame bytes.Buffer
	for {
		part, readErr := h.out.ReadSlice('\n')
		if frame.Len()+len(part) > 9<<20 {
			h.Close()
			return errors.New("cache_limit")
		}
		frame.Write(part)
		if readErr == nil {
			break
		}
		if readErr != bufio.ErrBufferFull {
			h.Close()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("script_process_failed")
		}
	}
	var reply struct {
		ID       uint64          `json:"id"`
		OK       bool            `json:"ok"`
		Value    json.RawMessage `json:"value"`
		Category string          `json:"category"`
	}
	d := json.NewDecoder(&frame)
	d.DisallowUnknownFields()
	if d.Decode(&reply) != nil || reply.ID != h.id {
		h.Close()
		return errors.New("script_protocol_failed")
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		h.Close()
		return errors.New("script_protocol_failed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !reply.OK {
		h.Close()
		switch reply.Category {
		case "proof_global_conflict", "proof_interpreter_failed", "proof_interpreter_missing", "proof_program_failed", "proof_snapshot_failed", "proof_snapshot_invalid", "proof_signals_missing", "script_memory_limit", "script_stack_limit", "script_timeout", "script_output_limit", "player_timestamp_missing", "player_function_missing", "script_reference_error", "script_type_error", "script_syntax_error", "script_internal_error", "script_range_error", "script_error", "script_conversion_failed", "script_json_failed", "script_encoding_failed", "script_result_type", "script_exception_value":
			return errors.New(reply.Category)
		}
		return errors.New("script_failed")
	}
	if len(reply.Value) == 0 || json.Unmarshal(reply.Value, out) != nil {
		h.Close()
		return errors.New("script_result_failed")
	}
	return nil
}
func jsJSON(value any) string { b, _ := json.Marshal(value); return string(b) }

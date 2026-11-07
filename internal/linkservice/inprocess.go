package linkservice

import (
	"context"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"
)

// InProcessFarEnd is how the receipt names the link service for a run of
// the app's own, which binds in its process.
const InProcessFarEnd = "Agentry Desktop's link service, in the app's own process"

// BindInProcess binds a run of the app's own and hands the engine the
// binding: calls and the unbind go to the service directly, as a CLI run's
// go over the link socket.
func (s *Service) BindInProcess(ctx context.Context, req desktop.BindRequest) (*engine.DesktopLink, error) {
	b, r := s.Bind(ctx, req)
	if r != nil {
		return nil, r
	}
	run, token := req.Run, b.Token
	return &engine.DesktopLink{
		Token:   token,
		Servers: b.Servers,
		Call: func(ctx context.Context, c desktop.CallRequest) (desktoplink.Answer, error) {
			a, r := s.Call(ctx, c)
			if r != nil {
				return desktoplink.Answer{}, r
			}
			return a, nil
		},
		Unbind: func(ctx context.Context) error {
			if r := s.Unbind(ctx, desktop.UnbindRequest{Run: run, Token: token}); r != nil {
				return r
			}
			return nil
		},
		FarEnd: InProcessFarEnd,
	}, nil
}

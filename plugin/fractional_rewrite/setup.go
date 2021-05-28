package fractional_rewrite

import (
	"context"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
)

func init() { plugin.Register("fractional_rewrite", setup) }

func setup(c *caddy.Controller) error {
	rewrite, err := fractionalRewriteParse(c)
	if err != nil {
		return plugin.Error("fractional_rewrite", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return fractionalRewrite{Next: next, Rule: rewrite}
	})

	return nil
}

func fractionalRewriteParse(c *caddy.Controller) (Rule, error) {
	c.Next()
	args := c.RemainingArgs()
	if len(args) != 4 {
		return nil, plugin.Error("fractional_rewrite", c.ArgErr())
	}
	ruleName := args[0]
	fraction, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return nil, plugin.Error("fractional_rewrite", c.Errf("expected floating point value but got %s", args[1]))
	}
	switch ruleName {
	case "suffix":
		return &suffixRule{
			args[2],
			args[3],
			fraction,
		}, nil
	default:
		return nil, plugin.Error("fractional_rewrite", c.Errf("unknown rule name %s", ruleName))
	}
}

type Rule interface {
	// Rewrite rewrites the current request.
	Rewrite(ctx context.Context, state request.Request)
}

type suffixRule struct {
	Suffix      string
	Replacement string
	Fraction    float64
}

// Rewrite rewrites the current request when the name ends with the matching string.
func (rule *suffixRule) Rewrite(ctx context.Context, state request.Request) {
	key := state.W.RemoteAddr().String()
	if strings.HasSuffix(state.Name(), rule.Suffix) && rule.shouldRewrite(key){
		state.Req.Question[0].Name = strings.TrimSuffix(state.Name(), rule.Suffix) + rule.Replacement
	}
}

func (rule *suffixRule) shouldRewrite(key string) bool {
	h := fnv.New32a()
	h.Write([]byte(key))
	if h.Sum32() % 100 <= uint32(rule.Fraction * 100){
		return true
	}
	return false
}

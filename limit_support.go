package jsontrans

import (
	"fmt"
	"strings"
)

// LimitSet is a bit set identifying fields of Limits.
type LimitSet uint16

const (
	LimitTimeout LimitSet = 1 << iota
	LimitExpressionBytes
	LimitDepth
	LimitSteps
	LimitCollectionItems
	LimitInputBytes
	LimitOutputBytes
	LimitRegexTimeout
)
const knownEvalLimits = LimitTimeout | LimitExpressionBytes | LimitDepth | LimitSteps | LimitCollectionItems | LimitInputBytes | LimitOutputBytes | LimitRegexTimeout

// EvalLimitSupport optionally declares per-evaluation limit fields a Program
// cannot accept. Without it, all fields retain their existing forwarding behavior.
// Wrappers snapshot this immutable declaration, reject explicit nonzero requests
// for unsupported fields, and omit those fields from synthesized EvalOptions.
// Common validation budgets still apply. Programs must also reject unsupported
// settings on direct Eval calls and must always honor context cancellation.
type EvalLimitSupport interface{ UnsupportedEvalLimits() LimitSet }

func unsupportedEvalLimits(program Program) (LimitSet, error) {
	if provider, ok := program.(EvalLimitSupport); ok {
		unsupported := provider.UnsupportedEvalLimits()
		if unsupported & ^knownEvalLimits != 0 {
			return 0, fmt.Errorf("%w: unknown evaluation limit flags", ErrUnsupported)
		}
		return unsupported, nil
	}
	return 0, nil
}
func specifiedLimits(l *Limits) (set LimitSet) {
	if l == nil {
		return 0
	}
	if l.Timeout != 0 {
		set |= LimitTimeout
	}
	if l.MaxExpressionBytes != 0 {
		set |= LimitExpressionBytes
	}
	if l.MaxDepth != 0 {
		set |= LimitDepth
	}
	if l.MaxSteps != 0 {
		set |= LimitSteps
	}
	if l.MaxCollectionItems != 0 {
		set |= LimitCollectionItems
	}
	if l.MaxInputBytes != 0 {
		set |= LimitInputBytes
	}
	if l.MaxOutputBytes != 0 {
		set |= LimitOutputBytes
	}
	if l.RegexTimeout != 0 {
		set |= LimitRegexTimeout
	}
	return
}
func rejectUnsupportedLimits(unsupported LimitSet, requested *Limits) error {
	if unsupported == 0 || requested == nil {
		return nil
	}
	bad := unsupported & specifiedLimits(requested)
	if bad == 0 {
		return nil
	}
	names := []string{}
	for _, field := range []struct {
		flag LimitSet
		name string
	}{
		{LimitTimeout, "Timeout"}, {LimitExpressionBytes, "MaxExpressionBytes"}, {LimitDepth, "MaxDepth"}, {LimitSteps, "MaxSteps"},
		{LimitCollectionItems, "MaxCollectionItems"}, {LimitInputBytes, "MaxInputBytes"}, {LimitOutputBytes, "MaxOutputBytes"}, {LimitRegexTimeout, "RegexTimeout"},
	} {
		if bad&field.flag != 0 {
			names = append(names, field.name)
		}
	}
	return fmt.Errorf("%w: evaluation limits %s", ErrUnsupported, strings.Join(names, ", "))
}
func supportedEvalLimits(l Limits, unsupported LimitSet) Limits {
	if unsupported&LimitTimeout != 0 {
		l.Timeout = 0
	}
	if unsupported&LimitExpressionBytes != 0 {
		l.MaxExpressionBytes = 0
	}
	if unsupported&LimitDepth != 0 {
		l.MaxDepth = 0
	}
	if unsupported&LimitSteps != 0 {
		l.MaxSteps = 0
	}
	if unsupported&LimitCollectionItems != 0 {
		l.MaxCollectionItems = 0
	}
	if unsupported&LimitInputBytes != 0 {
		l.MaxInputBytes = 0
	}
	if unsupported&LimitOutputBytes != 0 {
		l.MaxOutputBytes = 0
	}
	if unsupported&LimitRegexTimeout != 0 {
		l.RegexTimeout = 0
	}
	return l
}

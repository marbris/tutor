package stats

import (
	"math"
	"strings"

	"ttr/internal/deck"
)

// ── Narrowing by more than one category ─────────────────────────

// Op is how a clause joins what came before it.
type Op int

const (
	And Op = iota
	Or
	// AndNot keeps what came before and drops the cards in this category.
	// As the first clause there is nothing before it, so it is simply NOT.
	AndNot
)

func (o Op) Symbol() string {
	switch o {
	case Or:
		return "∨"
	case AndNot:
		return "∧¬"
	}
	return "∧"
}

// Clause is one category in a narrowing, and how it joins the ones before.
type Clause struct {
	Op  Op
	Row Row
}

// Expr is a narrowing built a category at a time. It folds from the left —
// ((c0 op1 c1) op2 c2) … — which is the order the categories were added in,
// so the expression reads the way it was built. The first clause's Op only
// matters when it is AndNot: one category on its own is neither AND nor OR,
// but it can still be negated.
type Expr []Clause

// Match reports whether a card survives the narrowing. An empty expression
// narrows nothing.
func (e Expr) Match(c deck.Card) bool {
	if len(e) == 0 {
		return true
	}
	ok := e[0].Row.Match(c)
	if e[0].Op == AndNot {
		ok = !ok
	}
	for _, cl := range e[1:] {
		switch cl.Op {
		case Or:
			ok = ok || cl.Row.Match(c)
		case AndNot:
			ok = ok && !cl.Row.Match(c)
		default:
			ok = ok && cl.Row.Match(c)
		}
	}
	return ok
}

// Has reports whether a category is already part of the narrowing.
func (e Expr) Has(r Row) bool {
	return e.Index(r) >= 0
}

// Index is where a category sits in the narrowing, or -1.
func (e Expr) Index(r Row) int {
	for i, cl := range e {
		if cl.Row.Same(&r) {
			return i
		}
	}
	return -1
}

// Add appends a category, unless it's already there.
func (e Expr) Add(op Op, r Row) (Expr, bool) {
	if e.Has(r) {
		return e, false
	}
	return append(append(Expr(nil), e...), Clause{Op: op, Row: r}), true
}

// Toggle is alt+a, alt+o and alt+n: a category not in the narrowing is
// added with op; one already in with op is taken out; one in with another op
// takes this one, keeping its place. So pressing the same key twice always
// undoes it. The first category's And and Or are the same thing, so there
// either one counts as op.
func (e Expr) Toggle(op Op, r Row) Expr {
	i := e.Index(r)
	if i < 0 {
		out, _ := e.Add(op, r)
		return out
	}
	same := e[i].Op == op
	if i == 0 && op != AndNot && e[0].Op != AndNot {
		same = true
	}
	if same {
		return e.Without(r)
	}
	out := append(Expr(nil), e...)
	out[i].Op = op
	return out
}

// Without drops one category from the narrowing, leaving the rest as they
// were. If it was the first, the next one starts afresh and its own op is
// simply ignored.
func (e Expr) Without(r Row) Expr {
	var out Expr
	for _, cl := range e {
		if !cl.Row.Same(&r) {
			out = append(out, cl)
		}
	}
	return out
}

// String is the expression as it would be written:
// ((Creature ∧ Black) ∨ Artifact) ∧ 5, with a negated first category
// written ¬Black.
func (e Expr) String() string {
	if len(e) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.Repeat("(", max(len(e)-2, 0)))
	if e[0].Op == AndNot {
		b.WriteString("¬")
	}
	b.WriteString(e[0].Row.FilterName())
	for i, cl := range e[1:] {
		b.WriteString(" " + cl.Op.Symbol() + " " + cl.Row.FilterName())
		if i < len(e)-2 {
			b.WriteString(")")
		}
	}
	return b.String()
}

// ── Opening-hand odds ───────────────────────────────────────────

// HandSize is the opening hand the odds are drawn for.
const HandSize = 7

// AtLeast is the chance of drawing at least want of the succ cards that
// count, in draws cards from a deck of pop — the hypergeometric tail.
func AtLeast(pop, succ, want, draws int) float64 {
	if want <= 0 {
		return 1
	}
	if pop <= 0 || succ <= 0 {
		return 0
	}
	draws = min(draws, pop)
	succ = min(succ, pop)
	total := lchoose(pop, draws)
	p := 0.0
	for k := want; k <= min(draws, succ); k++ {
		if draws-k > pop-succ {
			continue
		}
		p += math.Exp(lchoose(succ, k) + lchoose(pop-succ, draws-k) - total)
	}
	return math.Min(p, 1)
}

// lchoose is log(n choose k).
func lchoose(n, k int) float64 {
	a, _ := math.Lgamma(float64(n + 1))
	b, _ := math.Lgamma(float64(k + 1))
	c, _ := math.Lgamma(float64(n - k + 1))
	return a - b - c
}

package indicator

import "time"

type direction int

const (
	neutral direction = iota
	higherBetter
	lowerBetter
)

type Field struct {
	Name      string
	Baseline  string
	Treatment string
	Verdict   string
}

type Comparison struct {
	Baseline  *Report
	Treatment *Report
}

func Compare(baseline, treatment *Report) Comparison {
	return Comparison{Baseline: baseline, Treatment: treatment}
}

func (c Comparison) Fields() []Field {
	observable := c.Baseline.FaultApplied && c.Treatment.FaultApplied
	fields := []Field{
		boolField("fault applied", c.Baseline.FaultApplied, c.Treatment.FaultApplied, lowerBetter),
		boolField("reachable", c.Baseline.Reachable, c.Treatment.Reachable, neutral),
		boolField("data exposed", c.Baseline.DataExposed, c.Treatment.DataExposed, lowerBetter),
		detectedField(c.Baseline, c.Treatment, observable),
		mttdField(c.Baseline, c.Treatment, observable),
		durField("recovery latency", c.Baseline.RecoveryLatency, c.Treatment.RecoveryLatency, neutral),
	}
	if c.Baseline.RecoveryIntegrity != nil || c.Treatment.RecoveryIntegrity != nil {
		fields = append(fields, optBoolField("recovery integrity",
			c.Baseline.RecoveryIntegrity, c.Treatment.RecoveryIntegrity))
	}
	return fields
}

func optBoolField(name string, b, t *bool) Field {
	f := Field{Name: name, Baseline: optBoolStr(b), Treatment: optBoolStr(t)}
	if b == nil || t == nil {
		f.Verdict = "—"
		return f
	}
	f.Verdict = verdict(boolScore(*t)-boolScore(*b), higherBetter)
	return f
}

func optBoolStr(v *bool) string {
	if v == nil {
		return "n/a"
	}
	return boolStr(*v)
}

func boolField(name string, b, t bool, dir direction) Field {
	return Field{
		Name:      name,
		Baseline:  boolStr(b),
		Treatment: boolStr(t),
		Verdict:   verdict(boolScore(t)-boolScore(b), dir),
	}
}

func detectedField(b, t *Report, observable bool) Field {
	f := Field{
		Name:      "detected",
		Baseline:  appliedBoolStr(b.FaultApplied, b.Detected),
		Treatment: appliedBoolStr(t.FaultApplied, t.Detected),
	}
	if !observable {
		f.Verdict = "—"
		return f
	}
	f.Verdict = verdict(boolScore(t.Detected)-boolScore(b.Detected), higherBetter)
	return f
}

func mttdField(b, t *Report, observable bool) Field {
	f := Field{
		Name:      "mttd",
		Baseline:  appliedMTTDStr(b.FaultApplied, b.MTTD),
		Treatment: appliedMTTDStr(t.FaultApplied, t.MTTD),
	}
	if !observable {
		f.Verdict = "—"
		return f
	}
	switch {
	case b.MTTD == nil && t.MTTD == nil:
		f.Verdict = "same"
	case b.MTTD == nil:
		f.Verdict = "improved"
	case t.MTTD == nil:
		f.Verdict = "regressed"
	default:
		f.Verdict = verdict(sign(int64(*t.MTTD-*b.MTTD)), lowerBetter)
	}
	return f
}

func appliedBoolStr(applied, v bool) string {
	if !applied {
		return "n/a"
	}
	return boolStr(v)
}

func appliedMTTDStr(applied bool, d *time.Duration) string {
	if !applied {
		return "n/a"
	}
	return FormatMTTD(d)
}

func durField(name string, b, t time.Duration, dir direction) Field {
	return Field{
		Name:      name,
		Baseline:  b.String(),
		Treatment: t.String(),
		Verdict:   verdict(sign(int64(t-b)), dir),
	}
}

func verdict(sign int, dir direction) string {
	if dir == neutral {
		return "—"
	}
	if sign == 0 {
		return "same"
	}
	improved := (dir == higherBetter && sign > 0) || (dir == lowerBetter && sign < 0)
	if improved {
		return "improved"
	}
	return "regressed"
}

func FormatMTTD(d *time.Duration) string {
	if d == nil {
		return "∞ (never)"
	}
	return d.String()
}

func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func boolScore(v bool) int {
	if v {
		return 1
	}
	return 0
}

func sign(n int64) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

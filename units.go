package apollo

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

var decimalPattern = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?$`)

func decimal(s string) (*big.Rat, error) {
	if len(s) > 128 || !decimalPattern.MatchString(s) {
		return nil, fmt.Errorf("invalid decimal string %q", s)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, errors.New("invalid decimal")
	}
	return r, nil
}
func pow10(n int32) *big.Rat {
	v := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(abs32(n))), nil)
	if n < 0 {
		return new(big.Rat).SetFrac(big.NewInt(1), v)
	}
	return new(big.Rat).SetInt(v)
}
func abs32(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}
func validUnits(m Market) error {
	for _, e := range []int32{m.AtomicResolution, m.QuantumConversionExponent, m.PriceExponent} {
		if e < -38 || e > 38 {
			return errors.New("market exponent outside SDK supported range -38..38")
		}
	}
	if m.StepBaseQuantums <= 0 || m.SubticksPerTick <= 0 {
		return errors.New("market tick and step must be positive")
	}
	return nil
}
func scaledInteger(value string, exponent int32, step int64) (int64, error) {
	r, e := decimal(value)
	if e != nil {
		return 0, e
	}
	if r.Sign() <= 0 || step <= 0 {
		return 0, errors.New("value and step must be positive")
	}
	r.Mul(r, pow10(exponent))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, errors.New("value is off quantum or exceeds int64")
	}
	v := r.Num().Int64()
	if v <= 0 || v%step != 0 {
		return 0, errors.New("value is off market tick/step or below minimum")
	}
	return v, nil
}
func PriceToSubticks(price string, m Market) (int64, error) {
	if e := validUnits(m); e != nil {
		return 0, e
	}
	return scaledInteger(price, m.AtomicResolution-m.QuantumConversionExponent+6, int64(m.SubticksPerTick))
}
func SizeToQuantums(size string, m Market) (int64, error) {
	if e := validUnits(m); e != nil {
		return 0, e
	}
	return scaledInteger(size, -m.AtomicResolution, int64(m.StepBaseQuantums))
}
func exactString(r *big.Rat) string {
	// Values emitted by this SDK have finite decimal expansions.
	s := r.FloatString(100)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}
func SubticksToPrice(value int64, m Market) (string, error) {
	if e := validUnits(m); e != nil {
		return "", e
	}
	return exactString(new(big.Rat).Mul(new(big.Rat).SetInt64(value), pow10(m.QuantumConversionExponent-m.AtomicResolution-6))), nil
}
func QuantumsToSize(value int64, m Market) (string, error) {
	if e := validUnits(m); e != nil {
		return "", e
	}
	return exactString(new(big.Rat).Mul(new(big.Rat).SetInt64(value), pow10(m.AtomicResolution))), nil
}
func PriceUnitsToPrice(value int64, m Market) (string, error) {
	if e := validUnits(m); e != nil {
		return "", e
	}
	return exactString(new(big.Rat).Mul(new(big.Rat).SetInt64(value), pow10(m.PriceExponent))), nil
}
func USDToMicroUSD(amount string) (int64, error) {
	r, e := decimal(amount)
	if e != nil {
		return 0, e
	}
	r.Mul(r, pow10(6))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, errors.New("USD amount off micro-USD or overflows int64")
	}
	return r.Num().Int64(), nil
}
func LeverageToIMF(leverage string) (uint32, error) {
	r, e := decimal(leverage)
	if e != nil {
		return 0, e
	}
	if r.Cmp(big.NewRat(1, 1)) < 0 || r.Cmp(big.NewRat(1_000_000, 1)) > 0 {
		return 0, errors.New("leverage must be in 1..1000000")
	}
	r.Quo(big.NewRat(1_000_000, 1), r)
	q, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if rem.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsUint64() || q.Uint64() < 1 || q.Uint64() > 1_000_000 {
		return 0, errors.New("invalid IMF")
	}
	return uint32(q.Uint64()), nil
}
func marketProtection(reference string, isBuy bool, slippage string, m Market) (int64, error) {
	if e := validUnits(m); e != nil {
		return 0, e
	}
	price, e := decimal(reference)
	if e != nil || price.Sign() <= 0 {
		return 0, errors.New("positive reference price required")
	}
	s, e := decimal(slippage)
	if e != nil || s.Sign() < 0 || s.Cmp(big.NewRat(1, 1)) >= 0 {
		return 0, errors.New("slippage must be >=0 and <1")
	}
	factor := big.NewRat(1, 1)
	if isBuy {
		factor.Add(factor, s)
	} else {
		factor.Sub(factor, s)
	}
	price.Mul(price, factor)
	price.Mul(price, pow10(m.AtomicResolution-m.QuantumConversionExponent+6))
	price.Quo(price, new(big.Rat).SetInt64(int64(m.SubticksPerTick)))
	q, r := new(big.Int).QuoRem(price.Num(), price.Denom(), new(big.Int))
	if isBuy && r.Sign() != 0 {
		q.Add(q, big.NewInt(1))
	}
	q.Mul(q, big.NewInt(int64(m.SubticksPerTick)))
	if !q.IsInt64() || q.Sign() <= 0 {
		return 0, errors.New("market protection price invalid or overflows")
	}
	return q.Int64(), nil
}

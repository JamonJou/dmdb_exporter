/*
 * Copyright (c) 2000-2018, 达梦数据库有限公司.
 * All rights reserved.
 */
package dm

import (
	"dmdb_exporter/dm/util"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const (
	XDEC_MAX_PREC int = 38
	XDEC_SIZE         = 21

	FLAG_ZERO     int = 0x80
	FLAG_POSITIVE int = 0xC1
	FLAG_NEGTIVE  int = 0x3E
	EXP_MAX       int = 0xFF - 1 - FLAG_POSITIVE
	EXP_MIN       int = FLAG_NEGTIVE + 1 - 0x7F

	NUM_POSITIVE int = 1
	NUM_NEGTIVE  int = 101
)

type DmDecimal struct {
	sign   int
	weight int
	prec   int
	scale  int
	digits string
}

func FromBigInt(bigInt *big.Int) (*DmDecimal, error) {
	return newDecimal(bigInt, len(bigInt.String()), 0)
}
func (d DmDecimal) ToBigInt() *big.Int {
	if d.isZero() {
		return big.NewInt(0)
	}
	var digits = d.digits
	if d.sign < 0 {
		digits = "-" + digits
	}
	i1, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		fmt.Println("errrrrr")
	}
	if d.weight > 0 {
		i2, ok := new(big.Int).SetString("1"+strings.Repeat("0", d.weight), 10)
		if !ok {
			fmt.Println("errrrrr")
		}
		i1.Mul(i1, i2)
	} else if d.weight < 0 {
		i2, ok := new(big.Int).SetString("1"+strings.Repeat("0", -d.weight), 10)
		if !ok {
			fmt.Println("errrrrr")
		}
		i1.Quo(i1, i2)
	}
	return i1
}
func FromBigFloat(bigFloat *big.Float) (*DmDecimal, error) {
	return newDecimal(bigFloat, int(bigFloat.Prec()), int(bigFloat.Prec()))
}
func (d DmDecimal) ToBigFloat() *big.Float {
	if d.isZero() {
		return big.NewFloat(0.0)
	}
	var digits = d.digits
	if d.sign < 0 {
		digits = "-" + digits
	}
	f1, ok := new(big.Float).SetString(digits)
	if !ok {
		fmt.Println("errrrrr")
	}
	if d.weight > 0 {
		f2, ok := new(big.Float).SetString("1" + strings.Repeat("0", d.weight))
		if !ok {
			fmt.Println("errrrrr")
		}
		f1.Mul(f1, f2)
	} else if d.weight < 0 {
		f2, ok := new(big.Float).SetString("1" + strings.Repeat("0", -d.weight))
		if !ok {
			fmt.Println("errrrrr")
		}
		f1.Quo(f1, f2)
	}
	return f1
}

func (d *DmDecimal) Sign() int {
	return d.sign
}

func (dest *DmDecimal) Scan(src interface{}) error {
	switch src := src.(type) {
	case *DmDecimal:
		*dest = *src
		return nil
	default:
		return UNSUPPORTED_SCAN
	}
}

func newDecimal(dec interface{}, prec int, scale int) (*DmDecimal, error) {
	d := &DmDecimal{
		prec:  prec,
		scale: scale,
	}
	switch de := dec.(type) {
	case *big.Int:
		d.sign = de.Sign()

		if d.isZero() {
			return d, nil
		}
		str := de.String()

		if d.sign < 0 {
			str = str[1:]
		}

		if err := checkPrec(len(str), prec); err != nil {
			return d, err
		}
		i := 0
		istart := len(str) - 1

		for i = istart; i > 0; i-- {
			if str[i] != '0' {
				break
			}
		}
		str = str[:i+1]
		d.weight += istart - i

		if isOdd(d.weight) {
			str += "0"
			d.weight -= 1
		}
		if isOdd(len(str)) {
			str = "0" + str
		}
		d.digits = str
	case *big.Float:
		d.sign = de.Sign()

		if d.isZero() {
			return d, nil
		}
		str := de.Text('f', -1)

		if d.sign < 0 {
			str = str[1:]
		}

		pointIndex := strings.IndexByte(str, '.')
		i, istart, length := 0, 0, len(str)

		if pointIndex != -1 {
			if str[0] == '0' {

				istart = 2
				for i = istart; i < length; i++ {
					if str[i] != '0' {
						break
					}
				}
				str = str[i:]
				d.weight -= i - istart + len(str)
			} else {
				str = str[:pointIndex] + str[pointIndex+1:]
				d.weight -= length - pointIndex - 1
			}
		}

		length = len(str)
		istart = length - 1
		for i = istart; i > 0; i-- {
			if str[i] != '0' {
				break
			}
		}
		str = str[:i+1] + str[length:]
		d.weight += istart - i

		if isOdd(d.weight) {
			str += "0"
			d.weight -= 1
		}
		if isOdd(len(str)) {
			str = "0" + str
		}
		d.digits = str
	case []byte:
		return decodeDecimal(de, prec, scale)
	}
	return d, nil
}

func (d DmDecimal) encodeDecimal() ([]byte, error) {
	if d.isZero() {
		return []byte{byte(FLAG_ZERO)}, nil
	}
	exp := (d.weight+len(d.digits))/2 - 1
	if exp > EXP_MAX || exp < EXP_MIN {
		return nil, ECGO_DATA_TOO_LONG.throw()
	}
	validLen := len(d.digits)/2 + 1

	if d.sign < 0 && validLen >= XDEC_SIZE {
		validLen = XDEC_SIZE - 1
	} else if validLen > XDEC_SIZE {
		validLen = XDEC_SIZE
	}
	retLen := validLen
	if d.sign < 0 {
		retLen = validLen + 1
	}
	retBytes := make([]byte, retLen)
	if d.sign > 0 {
		retBytes[0] = byte(exp + FLAG_POSITIVE)
	} else {
		retBytes[0] = byte(FLAG_NEGTIVE - exp)
	}

	ibytes := 1
	for ichar := 0; ibytes < validLen; {
		digit1, err := strconv.Atoi(string(d.digits[ichar]))
		if err != nil {
			return nil, err
		}
		ichar++
		digit2, err := strconv.Atoi(string(d.digits[ichar]))
		ichar++
		if err != nil {
			return nil, err
		}

		digit := digit1*10 + digit2
		if d.sign > 0 {
			retBytes[ibytes] = byte(digit + NUM_POSITIVE)
		} else {
			retBytes[ibytes] = byte(NUM_NEGTIVE - digit)
		}
		ibytes++
	}
	if d.sign < 0 && ibytes < retLen {
		retBytes[ibytes] = 0x66
		ibytes++
	}
	if ibytes < retLen {
		retBytes[ibytes] = 0x00
	}
	return retBytes, nil
}

func decodeDecimal(values []byte, prec int, scale int) (*DmDecimal, error) {
	var decimal = &DmDecimal{
		prec:   prec,
		scale:  scale,
		sign:   0,
		weight: 0,
	}
	if values == nil || len(values) == 0 || len(values) > XDEC_SIZE {
		return nil, ECGO_FATAL_ERROR.throw()
	}
	if values[0] == byte(FLAG_ZERO) || len(values) == 1 {
		return decimal, nil
	}
	if values[0]&byte(FLAG_ZERO) != 0 {
		decimal.sign = 1
	} else {
		decimal.sign = -1
	}

	var flag = int(util.Dm_build_586.Dm_build_704(values, 0))
	var exp int
	if decimal.sign > 0 {
		exp = flag - FLAG_POSITIVE
	} else {
		exp = FLAG_NEGTIVE - flag
	}
	var digit = 0
	var sf = ""
	for ival := 1; ival < len(values); ival++ {
		if decimal.sign > 0 {
			digit = int(values[ival]) - NUM_POSITIVE
		} else {
			digit = NUM_NEGTIVE - int(values[ival])
		}
		if digit < 0 || digit > 99 {
			break
		}
		if digit < 10 {
			sf += "0"
		}
		sf += strconv.Itoa(digit)
	}
	decimal.digits = sf
	decimal.weight = exp*2 - (len(decimal.digits) - 2)

	return decimal, nil
}

func (d DmDecimal) isZero() bool {
	return d.sign == 0
}

func checkPrec(len int, prec int) error {
	if prec > 0 && len > prec || len > XDEC_MAX_PREC {
		return ECGO_DATA_TOO_LONG.throw()
	}
	return nil
}

func isOdd(val int) bool {
	return val%2 != 0
}

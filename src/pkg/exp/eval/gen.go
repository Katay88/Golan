// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// generate operator implementations

import (
	"log"
	"os"
	"template"
)

type Op struct {
	Name        string
	Expr        string
	Body        string // overrides Expr
	ConstExpr   string
	AsRightName string
	ReturnType  string
	Types       []*Type
}

type Size struct {
	Bits  int
	Sized string
}

type Type struct {
	Repr      string
	Value     string
	Native    string
	As        string
	IsIdeal   bool
	HasAssign bool
	Sizes     []Size
}

var (
	boolType = &Type{Repr: "*boolType", Value: "BoolValue", Native: "bool", As: "asBool"}
	uintType = &Type{Repr: "*uintType", Value: "UintValue", Native: "uint64", As: "asUint",
		Sizes: []Size{{8, "uint8"}, {16, "uint16"}, {32, "uint32"}, {64, "uint64"}, {0, "uint"}},
	}
	intType = &Type{Repr: "*intType", Value: "IntValue", Native: "int64", As: "asInt",
		Sizes: []Size{{8, "int8"}, {16, "int16"}, {32, "int32"}, {64, "int64"}, {0, "int"}},
	}
	idealIntType = &Type{Repr: "*idealIntType", Value: "IdealIntValue", Native: "*big.Int", As: "asIdealInt", IsIdeal: true}
	floatType    = &Type{Repr: "*floatType", Value: "FloatValue", Native: "float64", As: "asFloat",
		Sizes: []Size{{32, "float32"}, {64, "float64"}},
	}
	idealFloatType = &Type{Repr: "*idealFloatType", Value: "IdealFloatValue", Native: "*big.Rat", As: "asIdealFloat", IsIdeal: true}
	stringType     = &Type{Repr: "*stringType", Value: "StringValue", Native: "string", As: "asString"}
	arrayType      = &Type{Repr: "*ArrayType", Value: "ArrayValue", Native: "ArrayValue", As: "asArray", HasAssign: true}
	structType     = &Type{Repr: "*StructType", Value: "StructValue", Native: "StructValue", As: "asStruct", HasAssign: true}
	ptrType        = &Type{Repr: "*PtrType", Value: "PtrValue", Native: "Value", As: "asPtr"}
	funcType       = &Type{Repr: "*FuncType", Value: "FuncValue", Native: "Func", As: "asFunc"}
	sliceType      = &Type{Repr: "*SliceType", Value: "SliceValue", Native: "Slice", As: "asSlice"}
	mapType        = &Type{Repr: "*MapType", Value: "MapValue", Native: "Map", As: "asMap"}

	all = []*Type{
		boolType,
		uintType,
		intType,
		idealIntType,
		floatType,
		idealFloatType,
		stringType,
		arrayType,
		structType,
		ptrType,
		funcType,
		sliceType,
		mapType,
	}
	bools     = all[0:1]
	integers  = all[1:4]
	shiftable = all[1:3]
	numbers   = all[1:6]
	addable   = all[1:7]
	cmpable   = []*Type{
		boolType,
		uintType,
		intType,
		idealIntType,
		floatType,
		idealFloatType,
		stringType,
		ptrType,
		funcType,
		mapType,
	}
)

var unOps = []Op{
	{Name: "Neg", Expr: "-v", ConstExpr: "val.Neg(val)", Types: numbers},
	{Name: "Not", Expr: "!v", Types: bools},
	{Name: "Xor", Expr: "^v", ConstExpr: "val.Not(val)", Types: integers},
}

var binOps = []Op{
	{Name: "Add", Expr: "l + r", ConstExpr: "l.Add(l, r)", Types: addable},
	{Name: "Sub", Expr: "l - r", ConstExpr: "l.Sub(l, r)", Types: numbers},
	{Name: "Mul", Expr: "l * r", ConstExpr: "l.Mul(l, r)", Types: numbers},
	{Name: "Quo",
		Body:      "if r == 0 { t.Abort(DivByZeroError{}) }; ret =  l / r",
		ConstExpr: "l.Quo(l, r)",
		Types:     numbers,
	},
	{Name: "Rem",
		Body:      "if r == 0 { t.Abort(DivByZeroError{}) }; ret = l % r",
		ConstExpr: "l.Rem(l, r)",
		Types:     integers,
	},
	{Name: "And", Expr: "l & r", ConstExpr: "l.And(l, r)", Types: integers},
	{Name: "Or", Expr: "l | r", ConstExpr: "l.Or(l, r)", Types: integers},
	{Name: "Xor", Expr: "l ^ r", ConstExpr: "l.Xor(l, r)", Types: integers},
	{Name: "AndNot", Expr: "l &^ r", ConstExpr: "l.AndNot(l, r)", Types: integers},
	{Name: "Shl", Expr: "l << r", ConstExpr: "l.Lsh(l, uint(r.Value()))",
		AsRightName: "asUint", Types: shiftable,
	},
	{Name: "Shr", Expr: "l >> r", ConstExpr: "new(big.Int).Rsh(l, uint(r.Value()))",
		AsRightName: "asUint", Types: shiftable,
	},
	{Name: "Lss", Expr: "l < r", ConstExpr: "l.Cmp(r) < 0", ReturnType: "bool", Types: addable},
	{Name: "Gtr", Expr: "l > r", ConstExpr: "l.Cmp(r) > 0", ReturnType: "bool", Types: addable},
	{Name: "Leq", Expr: "l <= r", ConstExpr: "l.Cmp(r) <= 0", ReturnType: "bool", Types: addable},
	{Name: "Geq", Expr: "l >= r", ConstExpr: "l.Cmp(r) >= 0", ReturnType: "bool", Types: addable},
	{Name: "Eql", Expr: "l == r", ConstExpr: "l.Cmp(r) == 0", ReturnType: "bool", Types: cmpable},
	{Name: "Neq", Expr: "l != r", ConstExpr: "l.Cmp(r) != 0", ReturnType: "bool", Types: cmpable},
}

type Data struct {
	UnaryOps  []Op
	BinaryOps []Op
	Types     []*Type
}

var data = Data{
	unOps,
	binOps,
	all,
}

const templateStr = `
// This file is machine generated by gen.go.
// 6g gen.go && 6l gen.6 && ./6.out >expr1.go

package eval

import (
	"big"
	"log"
)

/*
 * "As" functions.  These retrieve evaluator functions from an
 * expr, panicking if the requested evaluator has the wrong type.
 */
«.repeated section Types»
«.section IsIdeal»
func (a *expr) «As»() (func() «Native») {
	return a.eval.(func()(«Native»))
}
«.or»
func (a *expr) «As»() (func(*Thread) «Native») {
	return a.eval.(func(*Thread)(«Native»))
}
«.end»
«.end»
func (a *expr) asMulti() (func(*Thread) []Value) {
	return a.eval.(func(*Thread)[]Value)
}

func (a *expr) asInterface() (func(*Thread) interface{}) {
	switch sf := a.eval.(type) {
«.repeated section Types»
«.section IsIdeal»
	case func()«Native»:
		return func(*Thread) interface{} { return sf() }
«.or»
	case func(t *Thread)«Native»:
		return func(t *Thread) interface{} { return sf(t) }
«.end»
«.end»
	default:
		log.Panicf("unexpected expression node type %T at %v", a.eval, a.pos)
	}
	panic("fail")
}

/*
 * Operator generators.
 */

func (a *expr) genConstant(v Value) {
	switch a.t.lit().(type) {
«.repeated section Types»
	case «Repr»:
«.section IsIdeal»
		val := v.(«Value»).Get()
		a.eval = func() «Native» { return val }
«.or»
		a.eval = func(t *Thread) «Native» { return v.(«Value»).Get(t) }
«.end»
«.end»
	default:
		log.Panicf("unexpected constant type %v at %v", a.t, a.pos)
	}
}

func (a *expr) genIdentOp(level, index int) {
	a.evalAddr = func(t *Thread) Value { return t.f.Get(level, index) }
	switch a.t.lit().(type) {
«.repeated section Types»
«.section IsIdeal»
«.or»
	case «Repr»:
		a.eval = func(t *Thread) «Native» { return t.f.Get(level, index).(«Value»).Get(t) }
«.end»
«.end»
	default:
		log.Panicf("unexpected identifier type %v at %v", a.t, a.pos)
	}
}

func (a *expr) genFuncCall(call func(t *Thread) []Value) {
	a.exec = func(t *Thread) { call(t)}
	switch a.t.lit().(type) {
«.repeated section Types»
«.section IsIdeal»
«.or»
	case «Repr»:
		a.eval = func(t *Thread) «Native» { return call(t)[0].(«Value»).Get(t) }
«.end»
«.end»
	case *MultiType:
		a.eval = func(t *Thread) []Value { return call(t) }
	default:
		log.Panicf("unexpected result type %v at %v", a.t, a.pos)
	}
}

func (a *expr) genValue(vf func(*Thread) Value) {
	a.evalAddr = vf
	switch a.t.lit().(type) {
«.repeated section Types»
«.section IsIdeal»
«.or»
	case «Repr»:
		a.eval = func(t *Thread) «Native» { return vf(t).(«Value»).Get(t) }
«.end»
«.end»
	default:
		log.Panicf("unexpected result type %v at %v", a.t, a.pos)
	}
}

«.repeated section UnaryOps»
func (a *expr) genUnaryOp«Name»(v *expr) {
	switch a.t.lit().(type) {
«.repeated section Types»
	case «Repr»:
«.section IsIdeal»
		val := v.«As»()()
		«ConstExpr»
		a.eval = func() «Native» { return val }
«.or»
		vf := v.«As»()
		a.eval = func(t *Thread) «Native» { v := vf(t); return «Expr» }
«.end»
«.end»
	default:
		log.Panicf("unexpected type %v at %v", a.t, a.pos)
	}
}

«.end»
func (a *expr) genBinOpLogAnd(l, r *expr) {
	lf := l.asBool()
	rf := r.asBool()
	a.eval = func(t *Thread) bool { return lf(t) && rf(t) }
}

func (a *expr) genBinOpLogOr(l, r *expr) {
	lf := l.asBool()
	rf := r.asBool()
	a.eval = func(t *Thread) bool { return lf(t) || rf(t) }
}

«.repeated section BinaryOps»
func (a *expr) genBinOp«Name»(l, r *expr) {
	switch t := l.t.lit().(type) {
«.repeated section Types»
	case «Repr»:
	«.section IsIdeal»
		l := l.«As»()()
		r := r.«As»()()
		val := «ConstExpr»
		«.section ReturnType»
		a.eval = func(t *Thread) «ReturnType» { return val }
		«.or»
		a.eval = func() «Native» { return val }
		«.end»
	«.or»
		lf := l.«As»()
		rf := r.«.section AsRightName»«@»«.or»«As»«.end»()
		«.section ReturnType»
		a.eval = func(t *Thread) «@» {
			l, r := lf(t), rf(t)
			return «Expr»
		}
		«.or»
		«.section Sizes»
		switch t.Bits {
		«.repeated section @»
		case «Bits»:
			a.eval = func(t *Thread) «Native» {
				l, r := lf(t), rf(t)
				var ret «Native»
				«.section Body»
				«Body»
				«.or»
				ret = «Expr»
				«.end»
				return «Native»(«Sized»(ret))
			}
		«.end»
		default:
			log.Panicf("unexpected size %d in type %v at %v", t.Bits, t, a.pos)
		}
		«.or»
		a.eval = func(t *Thread) «Native» {
			l, r := lf(t), rf(t)
			return «Expr»
		}
		«.end»
		«.end»
	«.end»
	«.end»
	default:
		log.Panicf("unexpected type %v at %v", l.t, a.pos)
	}
}

«.end»
func genAssign(lt Type, r *expr) (func(lv Value, t *Thread)) {
	switch lt.lit().(type) {
«.repeated section Types»
«.section IsIdeal»
«.or»
	case «Repr»:
		rf := r.«As»()
		return func(lv Value, t *Thread) { «.section HasAssign»lv.Assign(t, rf(t))«.or»lv.(«Value»).Set(t, rf(t))«.end» }
«.end»
«.end»
	default:
		log.Panicf("unexpected left operand type %v at %v", lt, r.pos)
	}
	panic("fail")
}
`

func main() {
	t := template.New(nil)
	t.SetDelims("«", "»")
	err := t.Parse(templateStr)
	if err != nil {
		log.Exit(err)
	}
	err = t.Execute(os.Stdout, data)
	if err != nil {
		log.Exit(err)
	}
}

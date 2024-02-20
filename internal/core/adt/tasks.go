// Copyright 2023 CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adt

import (
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/token"
)

var (
	handleExpr              *runner
	handleResolver          *runner
	handleDynamic           *runner
	handlePatternConstraint *runner
	handleComprehension     *runner
	handleListLit           *runner
	handleListVertex        *runner
	handleDisjunctions      *runner
)

// Use init to avoid a (spurious?) cyclic dependency in Go.
func init() {
	handleExpr = &runner{
		name:      "Expr",
		f:         processExpr,
		completes: genericConjunct,
	}
	handleResolver = &runner{
		name:      "Resolver",
		f:         processResolver,
		completes: genericConjunct,
	}
	handleDynamic = &runner{
		name:      "Dynamic",
		f:         processDynamic,
		completes: fieldConjunct,
	}
	handlePatternConstraint = &runner{
		name:      "PatternConstraint",
		f:         processPatternConstraint,
		completes: allTasksCompleted | fieldConjunctsKnown,
	}
	handleComprehension = &runner{
		name:      "Comprehension",
		f:         processComprehension,
		completes: valueKnown | allTasksCompleted | fieldConjunctsKnown,
	}
	handleListLit = &runner{
		name:      "ListLit",
		f:         processListLit,
		completes: fieldConjunct,
		needs:     listTypeKnown,
	}
	handleListVertex = &runner{
		name:      "ListVertex",
		f:         processListVertex,
		completes: fieldConjunct,
		needs:     listTypeKnown,
	}
	handleDisjunctions = &runner{
		name:      "Disjunctions",
		f:         processDisjunctions,
		completes: genericDisjunction,
		priority:  1,
	}
}

// This file contains task runners (func(ctx *OpContext, t *task, mode runMode)).

func processExpr(ctx *OpContext, t *task, mode runMode) {
	x := t.x.(Expr)

	state := combineMode(concreteKnown, mode)
	v := ctx.evalState(x, state)
	t.node.insertValueConjunct(t.env, v, t.id)
}

func processResolver(ctx *OpContext, t *task, mode runMode) {
	r := t.x.(Resolver)

	arc := r.resolve(ctx, oldOnly(0))
	if arc == nil {
		// TODO: yield instead?
		return
	}
	// TODO: consider moving after markCycle or removing.
	arc = arc.Indirect()

	// A reference that points to itself indicates equality. In that case
	// we are done computing and we can return the arc as is.
	ci, skip := t.node.markCycle(arc, t.env, r, t.id)
	if skip {
		return
	}

	c := MakeConjunct(t.env, t.x, ci)
	t.node.scheduleVertexConjuncts(c, arc, ci)
}

func processDynamic(ctx *OpContext, t *task, mode runMode) {
	n := t.node

	field := t.x.(*DynamicField)

	v := ctx.scalarValue(t, field.Key)
	if v == nil {
		return
	}

	if v.Concreteness() != Concrete {
		n.addBottom(&Bottom{
			Code: IncompleteError,
			Err: ctx.NewPosf(pos(field.Key),
				"key value of dynamic field must be concrete, found %v", v),
		})
		return
	}

	f := ctx.Label(field.Key, v)
	// TODO: remove this restriction.
	if f.IsInt() {
		n.addErr(ctx.NewPosf(pos(field.Key), "integer fields not supported"))
		return
	}

	c := MakeConjunct(t.env, field, t.id)
	c.CloseInfo.cc = nil
	n.insertArc(f, field.ArcType, c, t.id, true)
}

func processPatternConstraint(ctx *OpContext, t *task, mode runMode) {
	n := t.node

	field := t.x.(*BulkOptionalField)

	// Note that the result may be a disjunction. Be sure to not take the
	// default value as we want to retain the options of the disjunction.
	v := ctx.evalState(field.Filter, require(0, scalarKnown))
	if v == nil {
		return
	}

	n.insertPattern(v, MakeConjunct(t.env, t.x, t.id))
}

func processComprehension(ctx *OpContext, t *task, mode runMode) {
	n := t.node

	y := &envYield{
		envComprehension: t.comp,
		leaf:             t.leaf,
		env:              t.env,
		id:               t.id,
		expr:             t.x,
	}

	err := n.processComprehension(y, 0)
	t.err = CombineErrors(nil, t.err, err)
	t.comp.vertex.state.addBottom(err)
}

func processDisjunctions(c *OpContext, t *task, mode runMode) {
	n := t.node
	err := n.processDisjunctions()
	t.err = CombineErrors(nil, t.err, err)
}

func processFinalizeDisjunctions(c *OpContext, t *task, mode runMode) {
	n := t.node
	n.finalizeDisjunctions()
}

func processListLit(c *OpContext, t *task, mode runMode) {
	n := t.node

	l := t.x.(*ListLit)

	n.updateCyclicStatus(t.id)

	var ellipsis Node

	index := int64(0)
	hasComprehension := false
	for j, elem := range l.Elems {
		// TODO: Terminate early in case of runaway comprehension.

		switch x := elem.(type) {
		case *Comprehension:
			err := c.yield(nil, t.env, x, 0, func(e *Environment) {
				label, err := MakeLabel(x.Source(), index, IntLabel)
				n.addErr(err)
				index++
				c := MakeConjunct(e, x.Value, t.id)
				n.insertArc(label, ArcMember, c, t.id, true)
			})
			hasComprehension = true
			if err != nil {
				n.addBottom(err)
				return
			}

		case *Ellipsis:
			if j != len(l.Elems)-1 {
				n.addErr(c.Newf("ellipsis must be last element in list"))
				return
			}

			elem := x.Value
			if elem == nil {
				elem = &Top{}
			}

			c := MakeConjunct(t.env, elem, t.id)
			pat := &BoundValue{
				Op:    GreaterEqualOp,
				Value: n.ctx.NewInt64(index, x),
			}
			n.insertPattern(pat, c)
			ellipsis = x

		default:
			label, err := MakeLabel(x.Source(), index, IntLabel)
			n.addErr(err)
			index++
			c := MakeConjunct(t.env, x, t.id)
			n.insertArc(label, ArcMember, c, t.id, true)
		}

		if max := n.maxListLen; n.listIsClosed && int(index) > max {
			n.invalidListLength(max, len(l.Elems), n.maxNode, l)
			return
		}
	}

	isClosed := ellipsis == nil

	switch max := n.maxListLen; {
	case int(index) < max:
		if isClosed {
			n.invalidListLength(int(index), max, l, n.maxNode)
			return
		}

	case int(index) > max,
		isClosed && !n.listIsClosed,
		(isClosed == n.listIsClosed) && !hasComprehension:
		n.maxListLen = int(index)
		n.maxNode = l
		n.listIsClosed = isClosed
	}

	n.updateListType(l, t.id, isClosed, ellipsis)
}

func processListVertex(c *OpContext, t *task, mode runMode) {
	n := t.node

	l := t.x.(*Vertex)

	elems := l.Elems()
	isClosed := l.IsClosedList()

	// TODO: Share with code above.
	switch max := n.maxListLen; {
	case len(elems) < max:
		if isClosed {
			n.invalidListLength(len(elems), max, l, n.maxNode)
			return
		}

	case len(elems) > max:
		if n.listIsClosed {
			n.invalidListLength(max, len(elems), n.maxNode, l)
			return
		}
		n.listIsClosed = isClosed
		n.maxListLen = len(elems)
		n.maxNode = l

	case isClosed:
		n.listIsClosed = true
		n.maxNode = l
	}

	for _, a := range elems {
		if a.Conjuncts == nil {
			c := MakeRootConjunct(nil, a)
			n.insertArc(a.Label, ArcMember, c, CloseInfo{}, true)
			continue
		}
		for _, c := range a.Conjuncts {
			c.CloseInfo.cc = t.id.cc
			n.insertArc(a.Label, ArcMember, c, t.id, true)
		}
	}

	n.updateListType(l, t.id, isClosed, nil)
}

func (n *nodeContext) updateListType(list Expr, id CloseInfo, isClosed bool, ellipsis Node) {
	m, ok := n.node.BaseValue.(*ListMarker)
	if !ok {
		m = &ListMarker{
			IsOpen: true,
		}
		n.node.setValue(n.ctx, conjuncts, m)
	}
	m.IsOpen = m.IsOpen && !isClosed

	if ellipsis != nil {
		if src, _ := ellipsis.Source().(ast.Expr); src != nil {
			if m.Src == nil {
				m.Src = src
			} else {
				m.Src = ast.NewBinExpr(token.AND, m.Src, src)
			}
		}
	}

	if n.kind != ListKind {
		n.updateNodeType(ListKind, list, id)
	}
}

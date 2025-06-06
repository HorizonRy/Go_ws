// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package vta

import (
	"fmt"
	"go/token"
	"go/types"
	"iter"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/types/typeutil"
	"golang.org/x/tools/internal/typeparams"
)

// node interface for VTA nodes.
type node interface {
	Type() types.Type
	String() string
}

// constant node for VTA.
type constant struct {
	typ types.Type
}

func (c constant) Type() types.Type {
	return c.typ
}

func (c constant) String() string {
	return fmt.Sprintf("Constant(%v)", c.Type())
}

// pointer node for VTA.
type pointer struct {
	typ *types.Pointer
}

func (p pointer) Type() types.Type {
	return p.typ
}

func (p pointer) String() string {
	return fmt.Sprintf("Pointer(%v)", p.Type())
}

// mapKey node for VTA, modeling reachable map key types.
type mapKey struct {
	typ types.Type
}

func (mk mapKey) Type() types.Type {
	return mk.typ
}

func (mk mapKey) String() string {
	return fmt.Sprintf("MapKey(%v)", mk.Type())
}

// mapValue node for VTA, modeling reachable map value types.
type mapValue struct {
	typ types.Type
}

func (mv mapValue) Type() types.Type {
	return mv.typ
}

func (mv mapValue) String() string {
	return fmt.Sprintf("MapValue(%v)", mv.Type())
}

// sliceElem node for VTA, modeling reachable slice and array element types.
type sliceElem struct {
	typ types.Type
}

func (s sliceElem) Type() types.Type {
	return s.typ
}

func (s sliceElem) String() string {
	return fmt.Sprintf("Slice([]%v)", s.Type())
}

// channelElem node for VTA, modeling reachable channel element types.
type channelElem struct {
	typ types.Type
}

func (c channelElem) Type() types.Type {
	return c.typ
}

func (c channelElem) String() string {
	return fmt.Sprintf("Channel(chan %v)", c.Type())
}

// field node for VTA.
type field struct {
	StructType types.Type
	index      int // index of the field in the struct
}

func (f field) Type() types.Type {
	s := typeparams.CoreType(f.StructType).(*types.Struct)
	return s.Field(f.index).Type()
}

func (f field) String() string {
	s := typeparams.CoreType(f.StructType).(*types.Struct)
	return fmt.Sprintf("Field(%v:%s)", f.StructType, s.Field(f.index).Name())
}

// global node for VTA.
type global struct {
	val *ssa.Global
}

func (g global) Type() types.Type {
	return g.val.Type()
}

func (g global) String() string {
	return fmt.Sprintf("Global(%s)", g.val.Name())
}

// local node for VTA modeling local variables
// and function/method parameters.
type local struct {
	val ssa.Value
}

func (l local) Type() types.Type {
	return l.val.Type()
}

func (l local) String() string {
	return fmt.Sprintf("Local(%s)", l.val.Name())
}

// indexedLocal node for VTA node. Models indexed locals
// related to the ssa extract instructions.
type indexedLocal struct {
	val   ssa.Value
	index int
	typ   types.Type
}

func (i indexedLocal) Type() types.Type {
	return i.typ
}

func (i indexedLocal) String() string {
	return fmt.Sprintf("Local(%s[%d])", i.val.Name(), i.index)
}

// function node for VTA.
type function struct {
	f *ssa.Function
}

func (f function) Type() types.Type {
	return f.f.Type()
}

func (f function) String() string {
	return fmt.Sprintf("Function(%s)", f.f.Name())
}

// resultVar represents the result
// variable of a function, whether
// named or not.
type resultVar struct {
	f     *ssa.Function
	index int // valid index into result var tuple
}

func (o resultVar) Type() types.Type {
	return o.f.Signature.Results().At(o.index).Type()
}

func (o resultVar) String() string {
	v := o.f.Signature.Results().At(o.index)
	if n := v.Name(); n != "" {
		return fmt.Sprintf("Return(%s[%s])", o.f.Name(), n)
	}
	return fmt.Sprintf("Return(%s[%d])", o.f.Name(), o.index)
}

// nestedPtrInterface node represents all references and dereferences
// of locals and globals that have a nested pointer to interface type.
// We merge such constructs into a single node for simplicity and without
// much precision sacrifice as such variables are rare in practice. Both
// a and b would be represented as the same PtrInterface(I) node in:
//
//	type I interface
//	var a ***I
//	var b **I
type nestedPtrInterface struct {
	typ types.Type
}

func (l nestedPtrInterface) Type() types.Type {
	return l.typ
}

func (l nestedPtrInterface) String() string {
	return fmt.Sprintf("PtrInterface(%v)", l.typ)
}

// nestedPtrFunction node represents all references and dereferences of locals
// and globals that have a nested pointer to function type. We merge such
// constructs into a single node for simplicity and without much precision
// sacrifice as such variables are rare in practice. Both a and b would be
// represented as the same PtrFunction(func()) node in:
//
//	var a *func()
//	var b **func()
type nestedPtrFunction struct {
	typ types.Type
}

func (p nestedPtrFunction) Type() types.Type {
	return p.typ
}

func (p nestedPtrFunction) String() string {
	return fmt.Sprintf("PtrFunction(%v)", p.typ)
}

// panicArg models types of all arguments passed to panic.
type panicArg struct{}

func (p panicArg) Type() types.Type {
	return nil
}

func (p panicArg) String() string {
	return "Panic"
}

// recoverReturn models types of all return values of recover().
type recoverReturn struct{}

func (r recoverReturn) Type() types.Type {
	return nil
}

func (r recoverReturn) String() string {
	return "Recover"
}

type empty = struct{}

// idx is an index representing a unique node in a vtaGraph.
type idx int

// vtaGraph remembers for each VTA node the set of its successors.
// Tailored for VTA, hence does not support singleton (sub)graphs.
type vtaGraph struct {
	m    []map[idx]empty // m[i] has the successors for the node with index i.
	idx  map[node]idx    // idx[n] is the index for the node n.
	node []node          // node[i] is the node with index i.
}

func (g *vtaGraph) numNodes() int {
	return len(g.idx)
}

func (g *vtaGraph) successors(x idx) iter.Seq[idx] {
	return func(yield func(y idx) bool) {
		for y := range g.m[x] {
			if !yield(y) {
				return
			}
		}
	}
}

// addEdge adds an edge x->y to the graph.
func (g *vtaGraph) addEdge(x, y node) {
	if g.idx == nil {
		g.idx = make(map[node]idx)
	}
	lookup := func(n node) idx {
		i, ok := g.idx[n]
		if !ok {
			i = idx(len(g.idx))
			g.m = append(g.m, nil)
			g.idx[n] = i
			g.node = append(g.node, n)
		}
		return i
	}
	a := lookup(x)
	b := lookup(y)
	succs := g.m[a]
	if succs == nil {
		succs = make(map[idx]empty)
		g.m[a] = succs
	}
	succs[b] = empty{}
}

// typePropGraph builds a VTA graph for a set of `funcs` and initial
// `callgraph` needed to establish interprocedural edges. Returns the
// graph and a map for unique type representatives.
func typePropGraph(funcs map[*ssa.Function]bool, callees calleesFunc) (*vtaGraph, *typeutil.Map) {
	b := builder{callees: callees}
	b.visit(funcs)
	b.callees = nil // ensure callees is not pinned by pointers to other fields of b.
	return &b.graph, &b.canon
}

// Data structure responsible for linearly traversing the
// code and building a VTA graph.
type builder struct {
	graph   vtaGraph
	callees calleesFunc // initial call graph for creating flows at unresolved call sites.

	// Specialized type map for canonicalization of types.Type.
	// Semantically equivalent types can have different implementations,
	// i.e., they are different pointer values. The map allows us to
	// have one unique representative. The keys are fixed and from the
	// client perspective they are types. The values in our case are
	// types too, in particular type representatives. Each value is a
	// pointer so this map is not expected to take much memory.
	canon typeutil.Map
}

func (b *builder) visit(funcs map[*ssa.Function]bool) {
	// Add the fixed edge Panic -> Recover
	b.graph.addEdge(panicArg{}, recoverReturn{})

	for f, in := range funcs {
		if in {
			b.fun(f)
		}
	}
}

func (b *builder) fun(f *ssa.Function) {
	for _, bl := range f.Blocks {
		for _, instr := range bl.Instrs {
			b.instr(instr)
		}
	}
}

func (b *builder) instr(instr ssa.Instruction) {
	switch i := instr.(type) {
	case *ssa.Store:
		b.addInFlowAliasEdges(b.nodeFromVal(i.Addr), b.nodeFromVal(i.Val))
	case *ssa.MakeInterface:
		b.addInFlowEdge(b.nodeFromVal(i.X), b.nodeFromVal(i))
	case *ssa.MakeClosure:
		b.closure(i)
	case *ssa.UnOp:
		b.unop(i)
	case *ssa.Phi:
		b.phi(i)
	case *ssa.ChangeInterface:
		// Although in change interface a := A(b) command a and b are
		// the same object, the only interesting flow happens when A
		// is an interface. We create flow b -> a, but omit a -> b.
		// The latter flow is not needed: if a gets assigned concrete
		// type later on, that cannot be propagated back to b as b
		// is a separate variable. The a -> b flow can happen when
		// A is a pointer to interface, but then the command is of
		// type ChangeType, handled below.
		b.addInFlowEdge(b.nodeFromVal(i.X), b.nodeFromVal(i))
	case *ssa.ChangeType:
		// change type command a := A(b) results in a and b being the
		// same value. For concrete type A, there is no interesting flow.
		//
		// When A is an interface, most interface casts are handled
		// by the ChangeInterface instruction. The relevant case here is
		// when converting a pointer to an interface type. This can happen
		// when the underlying interfaces have the same method set.
		//
		//	type I interface{ foo() }
		//	type J interface{ foo() }
		//	var b *I
		//	a := (*J)(b)
		//
		// When this happens we add flows between a <--> b.
		b.addInFlowAliasEdges(b.nodeFromVal(i), b.nodeFromVal(i.X))
	case *ssa.TypeAssert:
		b.tassert(i)
	case *ssa.Extract:
		b.extract(i)
	case *ssa.Field:
		b.field(i)
	case *ssa.FieldAddr:
		b.fieldAddr(i)
	case *ssa.Send:
		b.send(i)
	case *ssa.Select:
		b.selekt(i)
	case *ssa.Index:
		b.index(i)
	case *ssa.IndexAddr:
		b.indexAddr(i)
	case *ssa.Lookup:
		b.lookup(i)
	case *ssa.MapUpdate:
		b.mapUpdate(i)
	case *ssa.Next:
		b.next(i)
	case ssa.CallInstruction:
		b.call(i)
	case *ssa.Panic:
		b.panic(i)
	case *ssa.Return:
		b.rtrn(i)
	case *ssa.MakeChan, *ssa.MakeMap, *ssa.MakeSlice, *ssa.BinOp,
		*ssa.Alloc, *ssa.DebugRef, *ssa.Convert, *ssa.Jump, *ssa.If,
		*ssa.Slice, *ssa.SliceToArrayPointer, *ssa.Range, *ssa.RunDefers:
		// No interesting flow here.
		// Notes on individual instructions:
		// SliceToArrayPointer: t1 = slice to array pointer *[4]T <- []T (t0)
		// No interesting flow as sliceArrayElem(t1) == sliceArrayElem(t0).
		return
	case *ssa.MultiConvert:
		b.multiconvert(i)
	default:
		panic(fmt.Sprintf("unsupported instruction %v\n", instr))
	}
}

func (b *builder) unop(u *ssa.UnOp) {
	switch u.Op {
	case token.MUL:
		// Multiplication operator * is used here as a dereference operator.
		b.addInFlowAliasEdges(b.nodeFromVal(u), b.nodeFromVal(u.X))
	case token.ARROW:
		t := typeparams.CoreType(u.X.Type()).(*types.Chan).Elem()
		b.addInFlowAliasEdges(b.nodeFromVal(u), channelElem{typ: t})
	default:
		// There is no interesting type flow otherwise.
	}
}

func (b *builder) phi(p *ssa.Phi) {
	for _, edge := range p.Edges {
		b.addInFlowAliasEdges(b.nodeFromVal(p), b.nodeFromVal(edge))
	}
}

func (b *builder) tassert(a *ssa.TypeAssert) {
	if !a.CommaOk {
		b.addInFlowEdge(b.nodeFromVal(a.X), b.nodeFromVal(a))
		return
	}
	// The case where a is <a.AssertedType, bool> register so there
	// is a flow from a.X to a[0]. Here, a[0] is represented as an
	// indexedLocal: an entry into local tuple register a at index 0.
	tup := a.Type().(*types.Tuple)
	t := tup.At(0).Type()

	local := indexedLocal{val: a, typ: t, index: 0}
	b.addInFlowEdge(b.nodeFromVal(a.X), local)
}

// extract instruction t1 := t2[i] generates flows between t2[i]
// and t1 where the source is indexed local representing a value
// from tuple register t2 at index i and the target is t1.
func (b *builder) extract(e *ssa.Extract) {
	tup := e.Tuple.Type().(*types.Tuple)
	t := tup.At(e.Index).Type()

	local := indexedLocal{val: e.Tuple, typ: t, index: e.Index}
	b.addInFlowAliasEdges(b.nodeFromVal(e), local)
}

func (b *builder) field(f *ssa.Field) {
	fnode := field{StructType: f.X.Type(), index: f.Field}
	b.addInFlowEdge(fnode, b.nodeFromVal(f))
}

func (b *builder) fieldAddr(f *ssa.FieldAddr) {
	t := typeparams.CoreType(f.X.Type()).(*types.Pointer).Elem()

	// Since we are getting pointer to a field, make a bidirectional edge.
	fnode := field{StructType: t, index: f.Field}
	b.addInFlowEdge(fnode, b.nodeFromVal(f))
	b.addInFlowEdge(b.nodeFromVal(f), fnode)
}

func (b *builder) send(s *ssa.Send) {
	t := typeparams.CoreType(s.Chan.Type()).(*types.Chan).Elem()
	b.addInFlowAliasEdges(channelElem{typ: t}, b.nodeFromVal(s.X))
}

// selekt generates flows for select statement
//
//	a = select blocking/nonblocking [c_1 <- t_1, c_2 <- t_2, ..., <- o_1, <- o_2, ...]
//
// between receiving channel registers c_i and corresponding input register t_i. Further,
// flows are generated between o_i and a[2 + i]. Note that a is a tuple register of type
// <int, bool, r_1, r_2, ...> where the type of r_i is the element type of channel o_i.
func (b *builder) selekt(s *ssa.Select) {
	recvIndex := 0
	for _, state := range s.States {
		t := typeparams.CoreType(state.Chan.Type()).(*types.Chan).Elem()

		if state.Dir == types.SendOnly {
			b.addInFlowAliasEdges(channelElem{typ: t}, b.nodeFromVal(state.Send))
		} else {
			// state.Dir == RecvOnly by definition of select instructions.
			tupEntry := indexedLocal{val: s, typ: t, index: 2 + recvIndex}
			b.addInFlowAliasEdges(tupEntry, channelElem{typ: t})
			recvIndex++
		}
	}
}

// index instruction a := b[c] on slices creates flows between a and
// SliceElem(t) flow where t is an interface type of c. Arrays and
// slice elements are both modeled as SliceElem.
func (b *builder) index(i *ssa.Index) {
	et := sliceArrayElem(i.X.Type())
	b.addInFlowAliasEdges(b.nodeFromVal(i), sliceElem{typ: et})
}

// indexAddr instruction a := &b[c] fetches address of a index
// into the field so we create bidirectional flow a <-> SliceElem(t)
// where t is an interface type of c. Arrays and slice elements are
// both modeled as SliceElem.
func (b *builder) indexAddr(i *ssa.IndexAddr) {
	et := sliceArrayElem(i.X.Type())
	b.addInFlowEdge(sliceElem{typ: et}, b.nodeFromVal(i))
	b.addInFlowEdge(b.nodeFromVal(i), sliceElem{typ: et})
}

// lookup handles map query commands a := m[b] where m is of type
// map[...]V and V is an interface. It creates flows between `a`
// and MapValue(V).
func (b *builder) lookup(l *ssa.Lookup) {
	t, ok := l.X.Type().Underlying().(*types.Map)
	if !ok {
		// No interesting flows for string lookups.
		return
	}

	if !l.CommaOk {
		b.addInFlowAliasEdges(b.nodeFromVal(l), mapValue{typ: t.Elem()})
	} else {
		i := indexedLocal{val: l, typ: t.Elem(), index: 0}
		b.addInFlowAliasEdges(i, mapValue{typ: t.Elem()})
	}
}

// mapUpdate handles map update commands m[b] = a where m is of type
// map[K]V and K and V are interfaces. It creates flows between `a`
// and MapValue(V) as well as between MapKey(K) and `b`.
func (b *builder) mapUpdate(u *ssa.MapUpdate) {
	t, ok := u.Map.Type().Underlying().(*types.Map)
	if !ok {
		// No interesting flows for string updates.
		return
	}

	b.addInFlowAliasEdges(mapKey{typ: t.Key()}, b.nodeFromVal(u.Key))
	b.addInFlowAliasEdges(mapValue{typ: t.Elem()}, b.nodeFromVal(u.Value))
}

// next instruction <ok, key, value> := next r, where r
// is a range over map or string generates flow between
// key and MapKey as well value and MapValue nodes.
func (b *builder) next(n *ssa.Next) {
	if n.IsString {
		return
	}
	tup := n.Type().(*types.Tuple)
	kt := tup.At(1).Type()
	vt := tup.At(2).Type()

	b.addInFlowAliasEdges(indexedLocal{val: n, typ: kt, index: 1}, mapKey{typ: kt})
	b.addInFlowAliasEdges(indexedLocal{val: n, typ: vt, index: 2}, mapValue{typ: vt})
}

// addInFlowAliasEdges adds an edge r -> l to b.graph if l is a node that can
// have an inflow, i.e., a node that represents an interface or an unresolved
// function value. Similarly for the edge l -> r with an additional condition
// of that l and r can potentially alias.
func (b *builder) addInFlowAliasEdges(l, r node) {
	b.addInFlowEdge(r, l)

	if canAlias(l, r) {
		b.addInFlowEdge(l, r)
	}
}

func (b *builder) closure(c *ssa.MakeClosure) {
	f := c.Fn.(*ssa.Function)
	b.addInFlowEdge(function{f: f}, b.nodeFromVal(c))

	for i, fv := range f.FreeVars {
		b.addInFlowAliasEdges(b.nodeFromVal(fv), b.nodeFromVal(c.Bindings[i]))
	}
}

// panic creates a flow from arguments to panic instructions to return
// registers of all recover statements in the program. Introduces a
// global panic node Panic and
//  1. for every panic statement p: add p -> Panic
//  2. for every recover statement r: add Panic -> r (handled in call)
//
// TODO(zpavlinovic): improve precision by explicitly modeling how panic
// values flow from callees to callers and into deferred recover instructions.
func (b *builder) panic(p *ssa.Panic) {
	// Panics often have, for instance, strings as arguments which do
	// not create interesting flows.
	if !canHaveMethods(p.X.Type()) {
		return
	}

	b.addInFlowEdge(b.nodeFromVal(p.X), panicArg{})
}

// call adds flows between arguments/parameters and return values/registers
// for both static and dynamic calls, as well as go and defer calls.
func (b *builder) call(c ssa.CallInstruction) {
	// When c is r := recover() call register instruction, we add Recover -> r.
	if bf, ok := c.Common().Value.(*ssa.Builtin); ok && bf.Name() == "recover" {
		if v, ok := c.(ssa.Value); ok {
			b.addInFlowEdge(recoverReturn{}, b.nodeFromVal(v))
		}
		return
	}

	for f := range siteCallees(c, b.callees) {
		addArgumentFlows(b, c, f)

		site, ok := c.(ssa.Value)
		if !ok {
			continue // go or defer
		}

		results := f.Signature.Results()
		if results.Len() == 1 {
			// When there is only one return value, the destination register does not
			// have a tuple type.
			b.addInFlowEdge(resultVar{f: f, index: 0}, b.nodeFromVal(site))
		} else {
			tup := site.Type().(*types.Tuple)
			for i := 0; i < results.Len(); i++ {
				local := indexedLocal{val: site, typ: tup.At(i).Type(), index: i}
				b.addInFlowEdge(resultVar{f: f, index: i}, local)
			}
		}
	}
}

func addArgumentFlows(b *builder, c ssa.CallInstruction, f *ssa.Function) {
	// When f has no paremeters (including receiver), there is no type
	// flow here. Also, f's body and parameters might be missing, such
	// as when vta is used within the golang.org/x/tools/go/analysis
	// framework (see github.com/golang/go/issues/50670).
	if len(f.Params) == 0 {
		return
	}
	cc := c.Common()
	if cc.Method != nil {
		// In principle we don't add interprocedural flows for receiver
		// objects. At a call site, the receiver object is interface
		// while the callee object is concrete. The flow from interface
		// to concrete type in general does not make sense. The exception
		// is when the concrete type is a named function type (see #57756).
		//
		// The flow other way around would bake in information from the
		// initial call graph.
		if isFunction(f.Params[0].Type()) {
			b.addInFlowEdge(b.nodeFromVal(cc.Value), b.nodeFromVal(f.Params[0]))
		}
	}

	offset := 0
	if cc.Method != nil {
		offset = 1
	}
	for i, v := range cc.Args {
		// Parameters of f might not be available, as in the case
		// when vta is used within the golang.org/x/tools/go/analysis
		// framework (see github.com/golang/go/issues/50670).
		//
		// TODO: investigate other cases of missing body and parameters
		if len(f.Params) <= i+offset {
			return
		}
		b.addInFlowAliasEdges(b.nodeFromVal(f.Params[i+offset]), b.nodeFromVal(v))
	}
}

// rtrn creates flow edges from the operands of the return
// statement to the result variables of the enclosing function.
func (b *builder) rtrn(r *ssa.Return) {
	for i, rs := range r.Results {
		b.addInFlowEdge(b.nodeFromVal(rs), resultVar{f: r.Parent(), index: i})
	}
}

func (b *builder) multiconvert(c *ssa.MultiConvert) {
	// TODO(zpavlinovic): decide what to do on MultiConvert long term.
	// TODO(zpavlinovic): add unit tests.
	typeSetOf := func(typ types.Type) []*types.Term {
		// This is a adaptation of x/exp/typeparams.NormalTerms which x/tools cannot depend on.
		var terms []*types.Term
		var err error
		switch typ := types.Unalias(typ).(type) {
		case *types.TypeParam:
			terms, err = typeparams.StructuralTerms(typ)
		case *types.Union:
			terms, err = typeparams.UnionTermSet(typ)
		case *types.Interface:
			terms, err = typeparams.InterfaceTermSet(typ)
		default:
			// Common case.
			// Specializing the len=1 case to avoid a slice
			// had no measurable space/time benefit.
			terms = []*types.Term{types.NewTerm(false, typ)}
		}

		if err != nil {
			return nil
		}
		return terms
	}
	// isValuePreserving returns true if a conversion from ut_src to
	// ut_dst is value-preserving, i.e. just a change of type.
	// Precondition: neither argument is a named or alias type.
	isValuePreserving := func(ut_src, ut_dst types.Type) bool {
		// Identical underlying types?
		if types.IdenticalIgnoreTags(ut_dst, ut_src) {
			return true
		}

		switch ut_dst.(type) {
		case *types.Chan:
			// Conversion between channel types?
			_, ok := ut_src.(*types.Chan)
			return ok

		case *types.Pointer:
			// Conversion between pointers with identical base types?
			_, ok := ut_src.(*types.Pointer)
			return ok
		}
		return false
	}
	dst_terms := typeSetOf(c.Type())
	src_terms := typeSetOf(c.X.Type())
	for _, s := range src_terms {
		us := s.Type().Underlying()
		for _, d := range dst_terms {
			ud := d.Type().Underlying()
			if isValuePreserving(us, ud) {
				// This is equivalent to a ChangeType.
				b.addInFlowAliasEdges(b.nodeFromVal(c), b.nodeFromVal(c.X))
				return
			}
			// This is equivalent to either: SliceToArrayPointer,,
			// SliceToArrayPointer+Deref, Size 0 Array constant, or a Convert.
		}
	}
}

// addInFlowEdge adds s -> d to g if d is node that can have an inflow, i.e., a node
// that represents an interface or an unresolved function value. Otherwise, there
// is no interesting type flow so the edge is omitted.
func (b *builder) addInFlowEdge(s, d node) {
	if hasInFlow(d) {
		b.graph.addEdge(b.representative(s), b.representative(d))
	}
}

// Creates const, pointer, global, func, and local nodes based on register instructions.
func (b *builder) nodeFromVal(val ssa.Value) node {
	if p, ok := types.Unalias(val.Type()).(*types.Pointer); ok && !types.IsInterface(p.Elem()) && !isFunction(p.Elem()) {
		// Nested pointer to interfaces are modeled as a special
		// nestedPtrInterface node.
		if i := interfaceUnderPtr(p.Elem()); i != nil {
			return nestedPtrInterface{typ: i}
		}
		// The same goes for nested function types.
		if f := functionUnderPtr(p.Elem()); f != nil {
			return nestedPtrFunction{typ: f}
		}
		return pointer{typ: p}
	}

	switch v := val.(type) {
	case *ssa.Const:
		return constant{typ: val.Type()}
	case *ssa.Global:
		return global{val: v}
	case *ssa.Function:
		return function{f: v}
	case *ssa.Parameter, *ssa.FreeVar, ssa.Instruction:
		// ssa.Param, ssa.FreeVar, and a specific set of "register" instructions,
		// satisifying the ssa.Value interface, can serve as local variables.
		return local{val: v}
	default:
		panic(fmt.Errorf("unsupported value %v in node creation", val))
	}
}

// representative returns a unique representative for node `n`. Since
// semantically equivalent types can have different implementations,
// this method guarantees the same implementation is always used.
func (b *builder) representative(n node) node {
	if n.Type() == nil {
		// panicArg and recoverReturn do not have
		// types and are unique by definition.
		return n
	}
	t := canonicalize(n.Type(), &b.canon)

	switch i := n.(type) {
	case constant:
		return constant{typ: t}
	case pointer:
		return pointer{typ: t.(*types.Pointer)}
	case sliceElem:
		return sliceElem{typ: t}
	case mapKey:
		return mapKey{typ: t}
	case mapValue:
		return mapValue{typ: t}
	case channelElem:
		return channelElem{typ: t}
	case nestedPtrInterface:
		return nestedPtrInterface{typ: t}
	case nestedPtrFunction:
		return nestedPtrFunction{typ: t}
	case field:
		return field{StructType: canonicalize(i.StructType, &b.canon), index: i.index}
	case indexedLocal:
		return indexedLocal{typ: t, val: i.val, index: i.index}
	case local, global, panicArg, recoverReturn, function, resultVar:
		return n
	default:
		panic(fmt.Errorf("canonicalizing unrecognized node %v", n))
	}
}

// canonicalize returns a type representative of `t` unique subject
// to type map `canon`.
func canonicalize(t types.Type, canon *typeutil.Map) types.Type {
	rep := canon.At(t)
	if rep != nil {
		return rep.(types.Type)
	}
	canon.Set(t, t)
	return t
}

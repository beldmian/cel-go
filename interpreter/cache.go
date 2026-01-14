// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package interpreter

import (
	"fmt"
	"hash"
	"hash/fnv"
	"sync"

	"github.com/google/cel-go/common/ast"
)

// InterpretableCache provides caching for Interpretable objects created during planning.
// This cache allows reusing expensive interpretable computations across evaluations when
// the AST structure is the same.
//
// Implementations must be safe for concurrent use.
type InterpretableCache interface {
	// Get retrieves a cached Interpretable for the given expression ID.
	// Returns the cached interpretable and true if found, nil and false otherwise.
	Get(exprID int64) (Interpretable, bool)

	// Put stores an Interpretable in the cache for the given expression ID.
	// If an entry already exists for this ID, it will be replaced.
	Put(exprID int64, interp Interpretable)

	// Clear removes all entries from the cache.
	Clear()

	// Size returns the number of entries in the cache.
	Size() int
}

// NewInterpretableCache creates a new thread-safe interpretable cache.
func NewInterpretableCache() InterpretableCache {
	return &syncMapCache{
		m: sync.Map{},
	}
}

// syncMapCache is a thread-safe implementation of InterpretableCache using sync.Map.
type syncMapCache struct {
	m sync.Map
}

func (c *syncMapCache) Get(exprID int64) (Interpretable, bool) {
	val, ok := c.m.Load(exprID)
	if !ok {
		return nil, false
	}
	return val.(Interpretable), true
}

func (c *syncMapCache) Put(exprID int64, interp Interpretable) {
	c.m.Store(exprID, interp)
}

func (c *syncMapCache) Clear() {
	c.m.Range(func(key, value interface{}) bool {
		c.m.Delete(key)
		return true
	})
}

func (c *syncMapCache) Size() int {
	count := 0
	c.m.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// HashExpr computes a stable hash for an AST expression.
// The hash is based on the expression structure, not just the ID.
func HashExpr(expr ast.Expr) uint64 {
	h := fnv.New64a()
	hashExprRecursive(expr, h)
	return h.Sum64()
}

func hashExprRecursive(expr ast.Expr, h hash.Hash64) {
	if expr == nil {
		return
	}

	// Hash the expression kind
	h.Write([]byte{byte(expr.Kind())})

	switch expr.Kind() {
	case ast.CallKind:
		call := expr.AsCall()
		h.Write([]byte(call.FunctionName()))
		if call.IsMemberFunction() {
			hashExprRecursive(call.Target(), h)
		}
		for _, arg := range call.Args() {
			hashExprRecursive(arg, h)
		}

	case ast.IdentKind:
		h.Write([]byte(expr.AsIdent()))

	case ast.LiteralKind:
		// Hash literal values
		lit := expr.AsLiteral()
		if lit != nil {
			// Use type name and value representation for hashing
			h.Write([]byte(lit.Type().TypeName()))
			// Use the Value() method which returns a string representation
			h.Write([]byte(fmt.Sprintf("%v", lit.Value())))
		}

	case ast.SelectKind:
		sel := expr.AsSelect()
		hashExprRecursive(sel.Operand(), h)
		h.Write([]byte(sel.FieldName()))
		if sel.IsTestOnly() {
			h.Write([]byte{1})
		}

	case ast.ListKind:
		list := expr.AsList()
		for _, elem := range list.Elements() {
			hashExprRecursive(elem, h)
		}
		// Hash optional indices
		for _, idx := range list.OptionalIndices() {
			h.Write([]byte{byte(idx >> 24), byte(idx >> 16), byte(idx >> 8), byte(idx)})
		}

	case ast.MapKind:
		m := expr.AsMap()
		for _, entry := range m.Entries() {
			me := entry.AsMapEntry()
			hashExprRecursive(me.Key(), h)
			hashExprRecursive(me.Value(), h)
			if me.IsOptional() {
				h.Write([]byte{1})
			}
		}

	case ast.StructKind:
		s := expr.AsStruct()
		h.Write([]byte(s.TypeName()))
		for _, field := range s.Fields() {
			sf := field.AsStructField()
			h.Write([]byte(sf.Name()))
			hashExprRecursive(sf.Value(), h)
			if sf.IsOptional() {
				h.Write([]byte{1})
			}
		}

	case ast.ComprehensionKind:
		comp := expr.AsComprehension()
		h.Write([]byte(comp.IterVar()))
		h.Write([]byte(comp.IterVar2()))
		h.Write([]byte(comp.AccuVar()))
		hashExprRecursive(comp.IterRange(), h)
		hashExprRecursive(comp.AccuInit(), h)
		hashExprRecursive(comp.LoopCondition(), h)
		hashExprRecursive(comp.LoopStep(), h)
		hashExprRecursive(comp.Result(), h)
	}
}

// PlannerPool provides pooling for intermediate planner objects to reduce allocations.
type PlannerPool struct {
	interpretableSlicePool *sync.Pool
}

// NewPlannerPool creates a new planner object pool.
func NewPlannerPool() *PlannerPool {
	return &PlannerPool{
		interpretableSlicePool: &sync.Pool{
			New: func() interface{} {
				// Pre-allocate slice with common capacity
				s := make([]Interpretable, 0, 8)
				return &s
			},
		},
	}
}

// GetInterpretableSlice retrieves a reusable interpretable slice from the pool.
func (p *PlannerPool) GetInterpretableSlice() *[]Interpretable {
	return p.interpretableSlicePool.Get().(*[]Interpretable)
}

// PutInterpretableSlice returns an interpretable slice to the pool.
func (p *PlannerPool) PutInterpretableSlice(s *[]Interpretable) {
	// Clear the slice but keep the capacity
	*s = (*s)[:0]
	p.interpretableSlicePool.Put(s)
}

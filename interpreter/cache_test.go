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
	"testing"

	"github.com/google/cel-go/common/ast"
	"github.com/google/cel-go/common/types"
)

func TestInterpretableCache_BasicOperations(t *testing.T) {
	cache := NewInterpretableCache()

	// Test empty cache
	if _, ok := cache.Get(1); ok {
		t.Error("Expected cache miss for empty cache")
	}
	if cache.Size() != 0 {
		t.Errorf("Expected size 0, got %d", cache.Size())
	}

	// Create a mock interpretable
	mockInterp := NewConstValue(1, types.True)

	// Test Put and Get
	cache.Put(1, mockInterp)
	if cache.Size() != 1 {
		t.Errorf("Expected size 1, got %d", cache.Size())
	}

	retrieved, ok := cache.Get(1)
	if !ok {
		t.Error("Expected cache hit after Put")
	}
	if retrieved != mockInterp {
		t.Error("Retrieved interpretable doesn't match stored value")
	}

	// Test overwrite
	mockInterp2 := NewConstValue(1, types.False)
	cache.Put(1, mockInterp2)
	if cache.Size() != 1 {
		t.Errorf("Expected size 1 after overwrite, got %d", cache.Size())
	}

	retrieved, ok = cache.Get(1)
	if !ok {
		t.Error("Expected cache hit after overwrite")
	}
	if retrieved != mockInterp2 {
		t.Error("Retrieved interpretable doesn't match overwritten value")
	}

	// Test multiple entries
	cache.Put(2, mockInterp)
	cache.Put(3, mockInterp2)
	if cache.Size() != 3 {
		t.Errorf("Expected size 3, got %d", cache.Size())
	}

	// Test Clear
	cache.Clear()
	if cache.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", cache.Size())
	}
	if _, ok := cache.Get(1); ok {
		t.Error("Expected cache miss after clear")
	}
}

func TestInterpretableCache_ConcurrentAccess(t *testing.T) {
	cache := NewInterpretableCache()
	const numGoroutines = 10
	const numOperations = 100

	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOperations; j++ {
				exprID := int64(id*numOperations + j)
				mockInterp := NewConstValue(exprID, types.IntZero)
				cache.Put(exprID, mockInterp)
			}
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOperations; j++ {
				exprID := int64(id*numOperations + j)
				_, _ = cache.Get(exprID)
			}
			done <- true
		}(i)
	}

	// Wait for all reads
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify final state
	expectedSize := numGoroutines * numOperations
	if cache.Size() != expectedSize {
		t.Errorf("Expected size %d after concurrent operations, got %d", expectedSize, cache.Size())
	}
}

func TestHashExpr_Consistency(t *testing.T) {
	fac := ast.NewExprFactory()
	tests := []struct {
		name     string
		buildAST func() ast.Expr
	}{
		{
			name: "literal",
			buildAST: func() ast.Expr {
				return fac.NewLiteral(1, types.IntZero)
			},
		},
		{
			name: "ident",
			buildAST: func() ast.Expr {
				return fac.NewIdent(2, "x")
			},
		},
		{
			name: "call",
			buildAST: func() ast.Expr {
				return fac.NewCall(3, "_==_",
					fac.NewIdent(4, "x"),
					fac.NewLiteral(5, types.IntZero))
			},
		},
		{
			name: "select",
			buildAST: func() ast.Expr {
				return fac.NewSelect(6,
					fac.NewIdent(7, "msg"),
					"field")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr1 := tt.buildAST()
			expr2 := tt.buildAST()

			hash1 := HashExpr(expr1)
			hash2 := HashExpr(expr2)

			if hash1 != hash2 {
				t.Errorf("Hash mismatch for identical expressions: %v != %v", hash1, hash2)
			}

			// Hash multiple times to ensure consistency
			for i := 0; i < 10; i++ {
				h := HashExpr(expr1)
				if h != hash1 {
					t.Errorf("Hash changed on iteration %d: %v != %v", i, h, hash1)
				}
			}
		})
	}
}

func TestHashExpr_Uniqueness(t *testing.T) {
	fac := ast.NewExprFactory()
	exprs := []ast.Expr{
		fac.NewLiteral(1, types.IntZero),
		fac.NewLiteral(2, types.IntOne),
		fac.NewIdent(3, "x"),
		fac.NewIdent(4, "y"),
		fac.NewCall(5, "_==_",
			fac.NewIdent(6, "x"),
			fac.NewLiteral(7, types.IntZero)),
		fac.NewCall(8, "_!=_",
			fac.NewIdent(9, "x"),
			fac.NewLiteral(10, types.IntZero)),
	}

	hashes := make(map[uint64]bool)
	for i, expr := range exprs {
		hash := HashExpr(expr)
		if hashes[hash] {
			t.Errorf("Hash collision detected for expression %d", i)
		}
		hashes[hash] = true
	}
}

func TestPlannerPool_InterpretableSlice(t *testing.T) {
	pool := NewPlannerPool()

	// Get slice from pool
	slice1 := pool.GetInterpretableSlice()
	if slice1 == nil {
		t.Fatal("Expected non-nil slice from pool")
	}
	if len(*slice1) != 0 {
		t.Errorf("Expected empty slice, got length %d", len(*slice1))
	}

	// Use the slice
	*slice1 = append(*slice1, NewConstValue(1, types.True))
	*slice1 = append(*slice1, NewConstValue(2, types.False))

	// Return to pool
	pool.PutInterpretableSlice(slice1)

	// Get another slice (should be the same one, now cleared)
	slice2 := pool.GetInterpretableSlice()
	if len(*slice2) != 0 {
		t.Errorf("Expected cleared slice from pool, got length %d", len(*slice2))
	}

	// Verify capacity is preserved
	if cap(*slice2) < 2 {
		t.Errorf("Expected capacity >= 2, got %d", cap(*slice2))
	}
}

func BenchmarkInterpretableCache_Get(b *testing.B) {
	cache := NewInterpretableCache()
	mockInterp := NewConstValue(1, types.True)

	// Populate cache
	for i := int64(0); i < 1000; i++ {
		cache.Put(i, mockInterp)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(int64(i % 1000))
	}
}

func BenchmarkInterpretableCache_Put(b *testing.B) {
	cache := NewInterpretableCache()
	mockInterp := NewConstValue(1, types.True)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(int64(i), mockInterp)
	}
}

func BenchmarkHashExpr_Simple(b *testing.B) {
	fac := ast.NewExprFactory()
	expr := fac.NewCall(1, "_==_",
		fac.NewIdent(2, "x"),
		fac.NewLiteral(3, types.IntZero))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashExpr(expr)
	}
}

func BenchmarkHashExpr_Complex(b *testing.B) {
	fac := ast.NewExprFactory()
	// Build a complex nested expression
	expr := fac.NewCall(1, "_&&_",
		fac.NewCall(2, "_==_",
			fac.NewSelect(3, fac.NewIdent(4, "msg"), "field1"),
			fac.NewLiteral(5, types.String("value"))),
		fac.NewCall(6, "_>_",
			fac.NewSelect(7, fac.NewIdent(8, "msg"), "field2"),
			fac.NewLiteral(9, types.IntZero)))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		HashExpr(expr)
	}
}

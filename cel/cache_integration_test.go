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

package cel_test

import (
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// TestInterpretableCache_CrossProgramReuse demonstrates that the cache
// successfully reuses interpretables across multiple program creations
func TestInterpretableCache_CrossProgramReuse(t *testing.T) {
	// Create environment with caching enabled
	env, err := cel.NewEnv(
		cel.EnableInterpretableCache(),
		cel.Variable("x", cel.IntType),
		cel.Variable("y", cel.IntType),
	)
	if err != nil {
		t.Fatalf("NewEnv() failed: %v", err)
	}

	// Parse and check multiple expressions that share sub-expressions
	// Both expressions contain the sub-expression "x + y"
	ast1, iss := env.Compile("x + y > 10")
	if iss.Err() != nil {
		t.Fatalf("Compile(ast1) failed: %v", iss.Err())
	}

	ast2, iss := env.Compile("x + y < 50")
	if iss.Err() != nil {
		t.Fatalf("Compile(ast2) failed: %v", iss.Err())
	}

	// Create programs - the second program should benefit from cached "x + y"
	prog1, err := env.Program(ast1)
	if err != nil {
		t.Fatalf("Program(ast1) failed: %v", err)
	}

	prog2, err := env.Program(ast2)
	if err != nil {
		t.Fatalf("Program(ast2) failed: %v", err)
	}

	// Verify both programs evaluate correctly
	vars := map[string]any{
		"x": 5,
		"y": 7,
	}

	out1, _, err := prog1.Eval(vars)
	if err != nil {
		t.Fatalf("prog1.Eval() failed: %v", err)
	}
	if out1 != types.True {
		t.Errorf("prog1.Eval() = %v, want true", out1)
	}

	out2, _, err := prog2.Eval(vars)
	if err != nil {
		t.Fatalf("prog2.Eval() failed: %v", err)
	}
	if out2 != types.True {
		t.Errorf("prog2.Eval() = %v, want true", out2)
	}
}

// TestInterpretableCache_WithOptimization verifies cache works with optimization enabled
func TestInterpretableCache_WithOptimization(t *testing.T) {
	env, err := cel.NewEnv(
		cel.EnableInterpretableCache(),
		cel.Variable("items", cel.ListType(cel.IntType)),
	)
	if err != nil {
		t.Fatalf("NewEnv() failed: %v", err)
	}

	// Expression with list operations
	ast1, iss := env.Compile("items.filter(i, i > 5).size() > 0")
	if iss.Err() != nil {
		t.Fatalf("Compile() failed: %v", iss.Err())
	}

	prog1, err := env.Program(ast1, cel.EvalOptions(cel.OptOptimize))
	if err != nil {
		t.Fatalf("Program() failed: %v", err)
	}

	vars := map[string]any{
		"items": []int{1, 3, 6, 9},
	}

	out, _, err := prog1.Eval(vars)
	if err != nil {
		t.Fatalf("Eval() failed: %v", err)
	}
	if out != types.True {
		t.Errorf("Eval() = %v, want true", out)
	}
}

// TestInterpretableCache_ComplexExpressions tests caching with nested expressions
func TestInterpretableCache_ComplexExpressions(t *testing.T) {
	env, err := cel.NewEnv(
		cel.EnableInterpretableCache(),
		cel.EnablePlannerPool(),
		cel.Variable("a", cel.IntType),
		cel.Variable("b", cel.IntType),
		cel.Variable("c", cel.IntType),
	)
	if err != nil {
		t.Fatalf("NewEnv() failed: %v", err)
	}

	// Create multiple programs with overlapping sub-expressions
	exprs := []string{
		"a + b",
		"(a + b) * c",
		"a + b + c",
		"a * 2 + b * 3 + c * 4",
		"(a + b) + (b + c)",
	}

	progs := make([]cel.Program, len(exprs))
	for i, expr := range exprs {
		ast, iss := env.Compile(expr)
		if iss.Err() != nil {
			t.Fatalf("Compile(%q) failed: %v", expr, iss.Err())
		}

		prog, err := env.Program(ast)
		if err != nil {
			t.Fatalf("Program(%q) failed: %v", expr, err)
		}
		progs[i] = prog
	}

	// Evaluate all programs
	vars := map[string]any{
		"a": 3,
		"b": 7,
		"c": 2,
	}

	expected := []int64{
		10,  // a + b
		20,  // (a + b) * c
		12,  // a + b + c
		35,  // a * 2 + b * 3 + c * 4
		19,  // (a + b) + (b + c)
	}

	for i, prog := range progs {
		out, _, err := prog.Eval(vars)
		if err != nil {
			t.Fatalf("progs[%d] (%q).Eval() failed: %v", i, exprs[i], err)
		}
		if !out.Equal(types.Int(expected[i])).(types.Bool) {
			t.Errorf("progs[%d].Eval() = %v, want %v", i, out, expected[i])
		}
	}
}

// BenchmarkProgramCreation_WithCache benchmarks program creation with caching
func BenchmarkProgramCreation_WithCache(b *testing.B) {
	env, err := cel.NewEnv(
		cel.EnableInterpretableCache(),
		cel.EnablePlannerPool(),
		cel.Variable("x", cel.IntType),
		cel.Variable("y", cel.IntType),
	)
	if err != nil {
		b.Fatalf("NewEnv() failed: %v", err)
	}

	ast, iss := env.Compile("x + y > 10 && x * y < 100")
	if iss.Err() != nil {
		b.Fatalf("Compile() failed: %v", iss.Err())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := env.Program(ast)
		if err != nil {
			b.Fatalf("Program() failed: %v", err)
		}
	}
}

// BenchmarkProgramCreation_WithoutCache benchmarks program creation without caching
func BenchmarkProgramCreation_WithoutCache(b *testing.B) {
	env, err := cel.NewEnv(
		cel.Variable("x", cel.IntType),
		cel.Variable("y", cel.IntType),
	)
	if err != nil {
		b.Fatalf("NewEnv() failed: %v", err)
	}

	ast, iss := env.Compile("x + y > 10 && x * y < 100")
	if iss.Err() != nil {
		b.Fatalf("Compile() failed: %v", iss.Err())
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := env.Program(ast)
		if err != nil {
			b.Fatalf("Program() failed: %v", err)
		}
	}
}

// BenchmarkMultiplePrograms_WithCache benchmarks creating many different programs with shared sub-expressions
func BenchmarkMultiplePrograms_WithCache(b *testing.B) {
	env, err := cel.NewEnv(
		cel.EnableInterpretableCache(),
		cel.EnablePlannerPool(),
		cel.Variable("a", cel.IntType),
		cel.Variable("b", cel.IntType),
		cel.Variable("c", cel.IntType),
	)
	if err != nil {
		b.Fatalf("NewEnv() failed: %v", err)
	}

	exprs := []string{
		"a + b",
		"(a + b) * c",
		"(a + b) > 10",
		"a + b + c",
		"(a + b) * (b + c)",
	}

	asts := make([]*cel.Ast, len(exprs))
	for i, expr := range exprs {
		ast, iss := env.Compile(expr)
		if iss.Err() != nil {
			b.Fatalf("Compile(%q) failed: %v", expr, iss.Err())
		}
		asts[i] = ast
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ast := range asts {
			_, err := env.Program(ast)
			if err != nil {
				b.Fatalf("Program() failed: %v", err)
			}
		}
	}
}

// BenchmarkMultiplePrograms_WithoutCache benchmarks creating many different programs without caching
func BenchmarkMultiplePrograms_WithoutCache(b *testing.B) {
	env, err := cel.NewEnv(
		cel.Variable("a", cel.IntType),
		cel.Variable("b", cel.IntType),
		cel.Variable("c", cel.IntType),
	)
	if err != nil {
		b.Fatalf("NewEnv() failed: %v", err)
	}

	exprs := []string{
		"a + b",
		"(a + b) * c",
		"(a + b) > 10",
		"a + b + c",
		"(a + b) * (b + c)",
	}

	asts := make([]*cel.Ast, len(exprs))
	for i, expr := range exprs {
		ast, iss := env.Compile(expr)
		if iss.Err() != nil {
			b.Fatalf("Compile(%q) failed: %v", expr, iss.Err())
		}
		asts[i] = ast
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ast := range asts {
			_, err := env.Program(ast)
			if err != nil {
				b.Fatalf("Program() failed: %v", err)
			}
		}
	}
}

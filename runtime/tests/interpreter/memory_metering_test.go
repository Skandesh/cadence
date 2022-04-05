/*
 * Cadence - The resource-oriented smart contract programming language
 *
 * Copyright 2022 Dapper Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package interpreter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/cadence/runtime/ast"
	"github.com/onflow/cadence/runtime/common"
	"github.com/onflow/cadence/runtime/interpreter"
	"github.com/onflow/cadence/runtime/sema"
	"github.com/onflow/cadence/runtime/stdlib"
	"github.com/onflow/cadence/runtime/tests/checker"
	"github.com/onflow/cadence/runtime/tests/utils"
)

type testMemoryGauge struct {
	meter map[common.MemoryKind]uint64
}

func newTestMemoryGauge() *testMemoryGauge {
	return &testMemoryGauge{
		meter: make(map[common.MemoryKind]uint64),
	}
}

func (g *testMemoryGauge) MeterMemory(usage common.MemoryUsage) error {
	g.meter[usage.Kind] += usage.Amount
	return nil
}

func (g *testMemoryGauge) getMemory(kind common.MemoryKind) uint64 {
	return g.meter[kind]
}

func TestInterpretArrayMetering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: [Int8] = []
                let y: [[String]] = [[]]
                let z: [[[Bool]]] = [[[]]]
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 1 for creation of x
		// 2 for creation of y
		// 1 for transfer of y
		// 1 dynamic type check of y
		// 3 for creation of z
		// 4 for transfer of z
		// 3 for dynamic type check of z
		// 14 from value transfer
		assert.Equal(t, uint64(29), meter.getMemory(common.MemoryKindArray))
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindVariable))
	})

	t.Run("iteration", func(t *testing.T) {
		t.Parallel()

		script := `
                pub fun main() {
                    let values: [[Int8]] = [[], [], []]
                    for value in values {
                      let a = value
                    }
                }
            `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(33), meter.getMemory(common.MemoryKindArray))
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindVariable))
	})

	t.Run("contains", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: [Int8] = []
                x.contains(5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretDictionaryMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: {Int8: String} = {}
                let y: {String: {Int8: String}} = {"a": {}}
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindString))
		assert.Equal(t, uint64(10), meter.getMemory(common.MemoryKindDictionary))
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindVariable))
	})

	t.Run("iteration", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let values: [{Int8: String}] = [{}, {}, {}]
                for value in values {
                  let a = value
                }
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(30), meter.getMemory(common.MemoryKindDictionary))
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindVariable))
	})

	t.Run("contains", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: {Int8: String} = {}
                x.containsKey(5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretCompositeMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct S {}

            pub resource R {
                pub let a: String
                pub let b: String

                init(a: String, b: String) {
                    self.a = a
                    self.b = b
                }
            }

            pub fun main() {
                let s = S()
                let r <- create R(a: "a", b: "b")
                destroy r
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(14), meter.getMemory(common.MemoryKindString))
		assert.Equal(t, uint64(66), meter.getMemory(common.MemoryKindRawString))
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindComposite))
		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindVariable))
	})

	t.Run("iteration", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct S {}

            pub fun main() {
                let values = [S(), S(), S()]
                for value in values {
                  let a = value
                }
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(30), meter.getMemory(common.MemoryKindComposite))
		assert.Equal(t, uint64(7), meter.getMemory(common.MemoryKindVariable))
	})
}

func TestInterpretCompositeFieldMetering(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		script := `
                    pub struct S {}
                    pub fun main() {
                        let s = S()
                    }
                `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindRawString))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindComposite))
	})

	t.Run("1 field", func(t *testing.T) {
		t.Parallel()

		script := `
                pub struct S {
                    pub let a: String
                    init(_ a: String) {
                        self.a = a
                    }
                }
                pub fun main() {
                    let s = S("a")
                }
            `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(16), meter.getMemory(common.MemoryKindRawString))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindComposite))
	})

	t.Run("2 field", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct S {
                pub let a: String
                pub let b: String
                init(_ a: String, _ b: String) {
                    self.a = a
                    self.b = b
                }
            }
            pub fun main() {
                let s = S("a", "b")
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(34), meter.getMemory(common.MemoryKindRawString))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindComposite))
	})
}

func TestInterpretInterpretedFunctionMetering(t *testing.T) {
	t.Parallel()

	t.Run("top level function", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {}
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindInterpretedFunction))
	})

	t.Run("function pointer creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let funcPointer = fun(a: String): String {
                    return a
                }
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 1 for the main, and 1 for the anon-func
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindInterpretedFunction))
	})

	t.Run("function pointer passing", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let funcPointer1 = fun(a: String): String {
                    return a
                }

                let funcPointer2 = funcPointer1
                let funcPointer3 = funcPointer2

                let value = funcPointer3("hello")
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 1 for the main, and 1 for the anon-func.
		// Assignment shouldn't allocate new memory, as the value is immutable and shouldn't be copied.
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindInterpretedFunction))
	})

	t.Run("struct method", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct Foo {
                pub fun bar() {}
            }

            pub fun main() {}
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 1 for the main, and 1 for the struct method.
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindInterpretedFunction))
	})

	t.Run("struct init", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct Foo {
                init() {}
            }

            pub fun main() {}
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 1 for the main, and 1 for the struct init.
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindInterpretedFunction))
	})
}

func TestInterpretHostFunctionMetering(t *testing.T) {
	t.Parallel()

	t.Run("top level function", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {}
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindHostFunction))
	})

	t.Run("function pointers", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let funcPointer1 = fun(a: String): String {
                    return a
                }

                let funcPointer2 = funcPointer1
                let funcPointer3 = funcPointer2

                let value = funcPointer3("hello")
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindHostFunction))
	})

	t.Run("struct method", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct Foo {
                pub fun bar() {}
            }

            pub fun main() {}
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 1 for the struct method.
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindHostFunction))
	})

	t.Run("struct init", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct Foo {
                init() {}
            }

            pub fun main() {}
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 1 for the struct init.
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindHostFunction))
	})

	t.Run("builtin functions", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let a = Int8(5)

                let b = CompositeType("PublicKey")
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// builtin functions are not metered
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindHostFunction))
	})

	t.Run("stdlib function", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                assert(true)
            }
        `

		meter := newTestMemoryGauge()
		inter, err := parseCheckAndInterpretWithOptionsAndMemoryMetering(
			t,
			script,
			ParseCheckAndInterpretOptions{
				CheckerOptions: []sema.Option{
					sema.WithPredeclaredValues(stdlib.BuiltinFunctions.ToSemaValueDeclarations()),
				},
				Options: []interpreter.Option{
					interpreter.WithPredeclaredValues(stdlib.BuiltinFunctions.ToInterpreterValueDeclarations()),
				},
			},
			meter,
		)
		require.NoError(t, err)

		_, err = inter.Invoke("main")
		require.NoError(t, err)

		// stdlib functions are not metered
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindHostFunction))
	})

	t.Run("public key creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let publicKey = PublicKey(
                    publicKey: "0102".decodeHex(),
                    signatureAlgorithm: SignatureAlgorithm.ECDSA_P256
                )
            }
        `

		var predeclaredSemaValues []sema.ValueDeclaration
		predeclaredSemaValues = append(predeclaredSemaValues, stdlib.BuiltinFunctions.ToSemaValueDeclarations()...)
		predeclaredSemaValues = append(predeclaredSemaValues, stdlib.BuiltinValues.ToSemaValueDeclarations()...)

		var predeclaredInterpreterValues []interpreter.ValueDeclaration
		predeclaredInterpreterValues = append(
			predeclaredInterpreterValues,
			stdlib.BuiltinFunctions.ToInterpreterValueDeclarations()...,
		)
		predeclaredInterpreterValues = append(
			predeclaredInterpreterValues,
			stdlib.BuiltinValues.ToInterpreterValueDeclarations()...,
		)

		meter := newTestMemoryGauge()
		inter, err := parseCheckAndInterpretWithOptionsAndMemoryMetering(
			t,
			script,
			ParseCheckAndInterpretOptions{
				CheckerOptions: []sema.Option{
					sema.WithPredeclaredValues(predeclaredSemaValues),
				},
				Options: []interpreter.Option{
					interpreter.WithPredeclaredValues(predeclaredInterpreterValues),
					interpreter.WithPublicKeyValidationHandler(
						func(_ *interpreter.Interpreter, _ func() interpreter.LocationRange, _ *interpreter.CompositeValue) error {
							return nil
						},
					),
				},
			},
			meter,
		)
		require.NoError(t, err)

		_, err = inter.Invoke("main")
		require.NoError(t, err)

		// 1 host function created for 'decodeHex' of String value
		// 'publicKeyVerify' and 'publicKeyVerifyPop' functions of PublicKey value are not metered
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindHostFunction))
	})

	t.Run("multiple public key creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let publicKey1 = PublicKey(
                    publicKey: "0102".decodeHex(),
                    signatureAlgorithm: SignatureAlgorithm.ECDSA_P256
                )

                let publicKey2 = PublicKey(
                    publicKey: "0102".decodeHex(),
                    signatureAlgorithm: SignatureAlgorithm.ECDSA_P256
                )
            }
        `

		var predeclaredSemaValues []sema.ValueDeclaration
		predeclaredSemaValues = append(predeclaredSemaValues, stdlib.BuiltinFunctions.ToSemaValueDeclarations()...)
		predeclaredSemaValues = append(predeclaredSemaValues, stdlib.BuiltinValues.ToSemaValueDeclarations()...)

		var predeclaredInterpreterValues []interpreter.ValueDeclaration
		predeclaredInterpreterValues = append(
			predeclaredInterpreterValues,
			stdlib.BuiltinFunctions.ToInterpreterValueDeclarations()...,
		)
		predeclaredInterpreterValues = append(
			predeclaredInterpreterValues,
			stdlib.BuiltinValues.ToInterpreterValueDeclarations()...,
		)

		meter := newTestMemoryGauge()
		inter, err := parseCheckAndInterpretWithOptionsAndMemoryMetering(
			t,
			script,
			ParseCheckAndInterpretOptions{
				CheckerOptions: []sema.Option{
					sema.WithPredeclaredValues(predeclaredSemaValues),
				},
				Options: []interpreter.Option{
					interpreter.WithPredeclaredValues(predeclaredInterpreterValues),
					interpreter.WithPublicKeyValidationHandler(
						func(_ *interpreter.Interpreter, _ func() interpreter.LocationRange, _ *interpreter.CompositeValue) error {
							return nil
						},
					),
				},
			},
			meter,
		)
		require.NoError(t, err)

		_, err = inter.Invoke("main")
		require.NoError(t, err)

		// 2 = 2x 1 host function created for 'decodeHex' of String value
		// 'publicKeyVerify' and 'publicKeyVerifyPop' functions of PublicKey value are not metered
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindHostFunction))
	})
}

func TestInterpretBoundFunctionMetering(t *testing.T) {
	t.Parallel()

	t.Run("struct method", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct Foo {
                pub fun bar() {}
            }

            pub fun main() {}
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// No bound functions are created without usages.
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindBoundFunction))
	})

	t.Run("struct init", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct Foo {
                init() {}
            }

            pub fun main() {}
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// No bound functions are created without usages.
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindBoundFunction))
	})

	t.Run("struct method usage", func(t *testing.T) {
		t.Parallel()

		script := `
            pub struct Foo {
                pub fun bar() {}
            }

            pub fun main() {
                let foo = Foo()
                foo.bar()
                foo.bar()
                foo.bar()
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 3 bound functions are created for the 3 invocations of 'bar()'.
		// No bound functions are created for init invocation.
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindBoundFunction))
	})
}

func TestInterpretOptionalValueMetering(t *testing.T) {
	t.Parallel()

	t.Run("simple optional value", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: String? = "hello"
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindOptional))
	})

	t.Run("dictionary get", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: {Int8: String} = {1: "foo", 2: "bar"}
                let y = x[0]
                let z = x[1]
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindOptional))
	})
}

func TestInterpretIntMetering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 + 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 16 = max(8, 8) + 8
		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 - 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 * 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 16 = 8 + 8
		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 / 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 % 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 | 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 ^ 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 & 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 << 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 16 = 8 + 8
		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 >> 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1
                let y = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8
		// result: 8 = 8
		assert.Equal(t, uint64(16), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretUIntMetering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8
		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt + 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 16 = max(8, 8) + 8
		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as UInt - 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt).saturatingSubtract(2 as UInt)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt * 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 16 = 8 + 8
		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt / 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt % 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt | 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt ^ 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt & 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt << 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 16 = 8 + 8
		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt >> 2 as UInt
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8 + 8
		// result: 8 = max(8, 8)
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1
                let y = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// TODO:
		// creation: 8
		// result: 8 = 8
		assert.Equal(t, uint64(16), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UInt = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretUInt8Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt8 + 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt8).saturatingAdd(2 as UInt8)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as UInt8 - 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt8).saturatingSubtract(2 as UInt8)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt8 * 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt8).saturatingMultiply(2 as UInt8)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt8 / 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt8 % 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt8 | 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt8 ^ 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt8 & 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt8 << 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt8 >> 2 as UInt8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UInt8 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretUInt16Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt16 + 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt16).saturatingAdd(2 as UInt16)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as UInt16 - 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt16).saturatingSubtract(2 as UInt16)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt16 * 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt16).saturatingMultiply(2 as UInt16)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt16 / 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt16 % 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt16 | 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt16 ^ 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt16 & 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt16 << 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt16 >> 2 as UInt16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UInt16 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretUInt32Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt32 + 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt32).saturatingAdd(2 as UInt32)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as UInt32 - 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt32).saturatingSubtract(2 as UInt32)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt32 * 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt32).saturatingMultiply(2 as UInt32)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt32 / 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt32 % 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt32 | 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt32 ^ 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt32 & 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt32 << 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt32 >> 2 as UInt32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UInt32 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretUInt64Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8
		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt64 + 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt64).saturatingAdd(2 as UInt64)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as UInt64 - 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt64).saturatingSubtract(2 as UInt64)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt64 * 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt64).saturatingMultiply(2 as UInt64)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt64 / 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt64 % 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt64 | 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt64 ^ 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt64 & 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt64 << 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt64 >> 2 as UInt64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UInt64 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretUInt128Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16
		assert.Equal(t, uint64(16), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt128 + 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt128).saturatingAdd(2 as UInt128)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as UInt128 - 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt128).saturatingSubtract(2 as UInt128)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt128 * 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt128).saturatingMultiply(2 as UInt128)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt128 / 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt128 % 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt128 | 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt128 ^ 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt128 & 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt128 << 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt128 >> 2 as UInt128
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UInt128 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretUInt256Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32
		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt256 + 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt256).saturatingAdd(2 as UInt256)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as UInt256 - 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt256).saturatingSubtract(2 as UInt256)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as UInt256 * 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = (1 as UInt256).saturatingMultiply(2 as UInt256)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt256 / 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt256 % 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt256 | 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt256 ^ 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt256 & 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt256 << 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as UInt256 >> 2 as UInt256
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UInt256 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretInt8Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1 + 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two operands (literals): 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1
                let y: Int8 = x.saturatingAdd(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1 - 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two operands (literals): 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1
                let y: Int8 = x.saturatingSubtract(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1 * 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two operands (literals): 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1
                let y: Int8 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 3 / 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two operands (literals): 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 3
                let y: Int8 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 3 % 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1
                let y: Int8 = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// x: 1
		// y: 1
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 3 | 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 3 ^ 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 3 & 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 3 << 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 3 >> 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int8 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretInt16Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1 + 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1
                let y: Int16 = x.saturatingAdd(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1 - 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1
                let y: Int16 = x.saturatingSubtract(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1 * 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1
                let y: Int16 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 3 / 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 3
                let y: Int16 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 3 % 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1
                let y: Int16 = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// x: 2
		// y: 2
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 3 | 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 3 ^ 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 3 & 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 3 << 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 3 >> 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int16 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretInt32Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1 + 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1
                let y: Int32 = x.saturatingAdd(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1 - 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1
                let y: Int32 = x.saturatingSubtract(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1 * 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1
                let y: Int32 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 3 / 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 3
                let y: Int32 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 3 % 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1
                let y: Int32 = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// x: 4
		// y: 4
		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 3 | 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 3 ^ 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 3 & 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 3 << 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 3 >> 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int32 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretInt64Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1 + 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1
                let y: Int64 = x.saturatingAdd(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1 - 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1
                let y: Int64 = x.saturatingSubtract(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1 * 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1
                let y: Int64 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 3 / 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 3
                let y: Int64 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 3 % 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1
                let y: Int64 = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// x: 8
		// y: 8
		assert.Equal(t, uint64(16), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 3 | 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 3 ^ 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 3 & 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 3 << 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 3 >> 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int64 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretInt128Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(16), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1 + 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1
                let y: Int128 = x.saturatingAdd(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1 - 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1
                let y: Int128 = x.saturatingSubtract(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1 * 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1
                let y: Int128 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 3 / 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 3
                let y: Int128 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 3 % 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1
                let y: Int128 = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// x: 16
		// y: 16
		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 3 | 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 3 ^ 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 3 & 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise left shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 3 << 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise right shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 3 >> 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 16 + 16
		// result: 16
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int128 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretInt256Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(32), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1 + 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1
                let y: Int256 = x.saturatingAdd(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1 - 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1
                let y: Int256 = x.saturatingSubtract(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1 * 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1
                let y: Int256 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 3 / 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("saturating division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 3
                let y: Int256 = x.saturatingMultiply(2)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 3 % 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1
                let y: Int256 = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// x: 32
		// y: 32
		assert.Equal(t, uint64(64), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 3 | 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 3 ^ 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 3 & 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise left shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 3 << 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("bitwise right shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 3 >> 2
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 32 + 32
		// result: 32
		assert.Equal(t, uint64(96), meter.getMemory(common.MemoryKindBigInt))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Int256 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretWord8Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word8 + 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as Word8 - 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word8 * 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word8 / 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word8 % 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word8 | 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word8 ^ 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word8 & 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word8 << 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word8 >> 2 as Word8
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 1 + 1
		// result: 1
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Word8 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretWord16Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word16 + 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as Word16 - 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word16 * 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word16 / 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word16 % 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word16 | 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word16 ^ 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word16 & 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word16 << 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word16 >> 2 as Word16
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 2 + 2
		// result: 2
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Word16 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretWord32Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word32 + 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as Word32 - 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word32 * 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word32 / 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word32 % 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word32 | 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word32 ^ 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word32 & 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word32 << 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word32 >> 2 as Word32
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 4 + 4
		// result: 4
		assert.Equal(t, uint64(12), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Word32 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretWord64Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8
		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word64 + 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 3 as Word64 - 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 1 as Word64 * 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word64 / 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word64 % 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise or", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word64 | 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise xor", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word64 ^ 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise and", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word64 & 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise left-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word64 << 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("bitwise right-shift", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x = 10 as Word64 >> 2 as Word64
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// creation: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Word64 = 1
                x == 1
                x != 1
                x > 1
                x >= 1
                x < 1
                x <= 1
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretBoolMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: Bool = true
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindBool))
	})

	t.Run("negation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                !true
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindBool))
	})

	t.Run("equality, true", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                true == true
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindBool))
	})

	t.Run("equality, false", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                true == false
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindBool))
	})

	t.Run("inequality", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                true != false
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretNilMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: Bool? = nil
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindNil))
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretVoidMetering(t *testing.T) {
	t.Parallel()

	t.Run("returnless function", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindVoid))
	})

	t.Run("returning function", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main(): Bool {
                return true
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindVoid))
	})
}

func TestInterpretStorageReferenceValueMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
              resource R {}

              pub fun main(account: AuthAccount) {
                  account.borrow<&R>(from: /storage/r)
              }
            `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		account := newTestAuthAccountValue(inter, interpreter.AddressValue{})
		_, err := inter.Invoke("main", account)
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindStorageReferenceValue))
	})
}

func TestInterpretEphemeralReferenceValueMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
          resource R {}

          pub fun main(): &Int {
              let x: Int = 1
              let y = &x as &Int
              return y
          }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindEphemeralReferenceValue))
	})

	t.Run("creation, optional", func(t *testing.T) {
		t.Parallel()

		script := `
          resource R {}

          pub fun main(): &Int {
              let x: Int? = 1
              let y = &x as &Int?
              return y!
          }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindEphemeralReferenceValue))
	})
}

func TestInterpretCharacterMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: Character = "a"
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// The lexer meters the literal "a" as a string.
		// To avoid double-counting, it is NOT metered as a Character as well.
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindCharacter))
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindString))
	})

	t.Run("assignment", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: Character = "a"
                let y = x
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// The lexer meters the literal "a" as a string.
		// To avoid double-counting, it is NOT metered as a Character as well.
		// Since characters are immutable, assigning them also does not allocate memory for them.
		assert.Equal(t, uint64(0), meter.getMemory(common.MemoryKindCharacter))
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindString))
	})

	t.Run("from string GetKey", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: String = "a"
                let y: Character = x[0]
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindCharacter))
	})
}

func TestInterpretAddressValueMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x: Address = 0x0
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindAddress))
	})

	t.Run("convert", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x = Address(0x0)
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindAddress))
	})
}

func TestInterpretPathValueMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x = /public/bar
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindPathValue))
	})

	t.Run("convert", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let x = PublicPath(identifier: "bar")
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindPathValue))
	})
}

func TestInterpretCapabilityValueMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            resource R {}

            pub fun main(account: AuthAccount) {
                let r <- create R()
                account.save(<-r, to: /storage/r)
                let x = account.link<&R>(/public/capo, target: /storage/r)
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		account := newTestAuthAccountValue(inter, interpreter.AddressValue{})
		_, err := inter.Invoke("main", account)
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindCapabilityValue))
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindPathValue))
	})
}

func TestInterpretLinkValueMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
            resource R {}

            pub fun main(account: AuthAccount) {
                account.link<&R>(/public/capo, target: /private/p)
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		account := newTestAuthAccountValue(inter, interpreter.AddressValue{})
		_, err := inter.Invoke("main", account)
		require.NoError(t, err)

		// Metered twice only when Atree validation is enabled.
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindLinkValue))
	})
}

func TestVariableMetering(t *testing.T) {
	t.Parallel()

	t.Run("globals", func(t *testing.T) {
		t.Parallel()

		script := `
            var a = 3
            let b = false

            pub fun main() {
                
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindVariable))
	})

	t.Run("params", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main(a: String, b: Bool) {
                
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main", interpreter.NewUnmeteredStringValue(""), interpreter.NewUnmeteredBoolValue(false))
		require.NoError(t, err)

		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindVariable))
	})

	t.Run("nested params", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                var x = fun (x: String, y: Bool) {}
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindVariable))
	})

	t.Run("applied nested params", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                var x = fun (x: String, y: Bool) {}
                x("", false)
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindVariable))
	})
}

func TestInterpretFix64Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.4
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.4 + 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.4
                let y: Fix64 = x.saturatingAdd(2.5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.4 - 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.4
                let y: Fix64 = x.saturatingSubtract(2.5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.4 * 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.4
                let y: Fix64 = x.saturatingMultiply(2.5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 3.4 / 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 3.4
                let y: Fix64 = x.saturatingMultiply(2.5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 3.4 % 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// quotient (div) : 8
		// truncatedQuotient: 8
		// truncatedQuotient.Mul(o): 8
		// result: 8
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("negation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.4
                let y: Fix64 = -x
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// x: 8
		// y: 8
		assert.Equal(t, uint64(16), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("creation as supertype", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: FixedPoint = -1.4
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: Fix64 = 1.0
                x == 1.0
                x != 1.0
                x > 1.0
                x >= 1.0
                x < 1.0
                x <= 1.0
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestInterpretUFix64Metering(t *testing.T) {

	t.Parallel()

	t.Run("creation", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 1.4
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 1.4 + 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating addition", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 1.4
                let y: UFix64 = x.saturatingAdd(2.5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 2.5 - 1.4 
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating subtraction", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 1.4
                let y: UFix64 = x.saturatingSubtract(2.5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 1.4 * 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating multiplication", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 1.4
                let y: UFix64 = x.saturatingMultiply(2.5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 3.4 / 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("saturating division", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 3.4
                let y: UFix64 = x.saturatingMultiply(2.5)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// result: 8
		assert.Equal(t, uint64(24), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("modulo", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 3.4 % 2.5
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// two literals: 8 + 8
		// quotient (div) : 8
		// truncatedQuotient: 8
		// truncatedQuotient.Mul(o): 8
		// result: 8
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("creation as supertype", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: FixedPoint = 1.4
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindNumber))
	})

	t.Run("logical operations", func(t *testing.T) {

		t.Parallel()

		script := `
            pub fun main() {
                let x: UFix64 = 1.0
                x == 1.0
                x != 1.0
                x > 1.0
                x >= 1.0
                x < 1.0
                x <= 1.0
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindBool))
	})
}

func TestTokenMetering(t *testing.T) {
	t.Parallel()

	t.Run("identifier tokens", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                var x: String = "hello"
            }

            pub struct foo {
                var x: Int

                init() {
                    self.x = 4
                }
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// keywords + func/var names
		assert.Equal(t, uint64(48), meter.getMemory(common.MemoryKindTokenIdentifier))
	})

	t.Run("syntax tokens", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                var a: [String] = []
                var b = 4 + 6
                var c = true && false != false
                var d = 4 as! AnyStruct
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)
		assert.Equal(t, uint64(21), meter.getMemory(common.MemoryKindTokenSyntax))
	})

	t.Run("comments", func(t *testing.T) {
		t.Parallel()

		script := `
            /*  first line
                second line
            */

            // single line comment
            pub fun main() {}
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// block comment start, end, (, ), {, }
		// Line comment start is not emitted
		assert.Equal(t, uint64(8), meter.getMemory(common.MemoryKindTokenSyntax))

		assert.Equal(t, uint64(75), meter.getMemory(common.MemoryKindTokenComment))
	})

	t.Run("numeric literals", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                var a = 1
                var b = 0b1
                var c = 0o1
                var d = 0x1
                var e = 1.4
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)
		assert.Equal(t, uint64(13), meter.getMemory(common.MemoryKindTokenNumericLiteral))
	})
}

func TestInterpreterStringLocationMetering(t *testing.T) {
	t.Parallel()

	t.Run("creation", func(t *testing.T) {
		t.Parallel()

		script := `
        struct S {}

        pub fun main(account: AuthAccount) {
            let s = CompositeType("S.test.S")
        }
		`
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)
		account := newTestAuthAccountValue(inter, interpreter.AddressValue{})
		_, err := inter.Invoke("main", account)
		require.NoError(t, err)

		// raw string location is "test"
		assert.Equal(t, uint64(5), meter.getMemory(common.MemoryKindRawString))
	})
}

func TestInterpretIdentifierMetering(t *testing.T) {
	t.Parallel()

	t.Run("variable", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                let foo = 4
                let bar = 5
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		// 'main', 'foo', 'bar', empty-return-type
		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindIdentifier))
	})

	t.Run("parameters", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main(foo: String, bar: String) {
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke(
			"main",
			interpreter.NewUnmeteredStringValue("x"),
			interpreter.NewUnmeteredStringValue("y"),
		)
		require.NoError(t, err)

		// 'main', 'foo', 'String', 'bar', 'String', empty-return-type
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindIdentifier))
	})

	t.Run("composite declaration", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {}

            pub struct foo {
                var x: String
                var y: String

                init() {
                    self.x = "a"
                    self.y = "b"
                }

                pub fun bar() {}
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)
		assert.Equal(t, uint64(16), meter.getMemory(common.MemoryKindIdentifier))
	})

	t.Run("member resolvers", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {            // 2 - 'main', empty-return-type
                let foo = ["a", "b"]    // 1
                foo.length              // 3 - 'foo', 'length', constant field resolver
                foo.length              // 3 - 'foo', 'length', constant field resolver (not re-used)
                foo.removeFirst()       // 3 - 'foo', 'removeFirst', function resolver
                foo.removeFirst()       // 3 - 'foo', 'removeFirst', function resolver (not re-used)
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)
		assert.Equal(t, uint64(15), meter.getMemory(common.MemoryKindIdentifier))
	})
}

func TestInterpretASTMetering(t *testing.T) {
	t.Parallel()

	t.Run("arguments", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                foo(a: "hello", b: 23)
                bar("hello", 23)
            }

            pub fun foo(a: String, b: Int) {
            }

            pub fun bar(_ a: String, _ b: Int) {
            }
        `
		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(4), meter.getMemory(common.MemoryKindArgument))
	})

	t.Run("blocks", func(t *testing.T) {
		script := `
            pub fun main() {
                var i = 0
                if i != 0 {
                    i = 0
                }

                while i < 2 {
                    i = i + 1
                }

                var a = "foo"
                switch i {
                    case 1:
                        a = "foo_1"
                    case 2:
                        a = "foo_2"
                    case 3:
                        a = "foo_3"
                }
            }
        `

		meter := newTestMemoryGauge()
		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)
		assert.Equal(t, uint64(7), meter.getMemory(common.MemoryKindBlock))
	})

	t.Run("declarations", func(t *testing.T) {
		script := `
            import Foo from 0x42

            pub let x = 1
            pub var y = 2

            pub fun main() {
                var z = 3
            }

            pub struct A {
                pub var a: String

                init() {
                    self.a = "hello"
                }
            }

            pub struct interface B {}

            pub resource C {
                let a: Int

                init() {
                    self.a = 6
                }
            }

            pub resource interface D {}

            pub enum E: Int8 {
                pub case a
                pub case b
                pub case c
            }

            transaction {}

            #pragma
        `

		importedChecker, err := checker.ParseAndCheckWithOptions(t,
			`
                pub let Foo = 1
            `,
			checker.ParseAndCheckOptions{
				Location: utils.ImportedLocation,
			},
		)
		require.NoError(t, err)

		meter := newTestMemoryGauge()
		inter, err := parseCheckAndInterpretWithOptionsAndMemoryMetering(
			t,
			script,
			ParseCheckAndInterpretOptions{
				CheckerOptions: []sema.Option{
					sema.WithImportHandler(
						func(_ *sema.Checker, _ common.Location, _ ast.Range) (sema.Import, error) {
							return sema.ElaborationImport{
								Elaboration: importedChecker.Elaboration,
							}, nil
						},
					),
				},
				Options: []interpreter.Option{
					interpreter.WithImportLocationHandler(
						func(inter *interpreter.Interpreter, location common.Location) interpreter.Import {
							require.IsType(t, common.AddressLocation{}, location)
							program := interpreter.ProgramFromChecker(importedChecker)
							subInterpreter, err := inter.NewSubInterpreter(program, location)
							if err != nil {
								panic(err)
							}

							return interpreter.InterpreterImport{
								Interpreter: subInterpreter,
							}
						},
					),
				},
			},
			meter,
		)
		require.NoError(t, err)

		_, err = inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindFunctionDeclaration))
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindCompositeDeclaration))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindInterfaceDeclaration))
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindEnumCaseDeclaration))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindFieldDeclaration))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindTransactionDeclaration))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindImportDeclaration))
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindVariableDeclaration))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindSpecialFunctionDeclaration))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindPragmaDeclaration))
	})

	t.Run("statements", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                var a = 5

                while a < 10 {               // while
                    if a == 5 {              // if
                        a = a + 1            // assignment
                        continue             // continue
                    }
                    break                    // break
                }

                foo()                        // expression statement

                for value in [1, 2, 3] {}    // for

                var r1 <- create bar()
                var r2 <- create bar()
                r1 <-> r2                    // swap

                destroy r1                   // expression statement
                destroy r2                   // expression statement

                switch a {                   // switch
                    case 1:
                        a = 2                // assignment
                }
            }

            pub fun foo(): Int {
                 return 5                    // return
            }

            resource bar {}

            pub contract Events {
                event FooEvent(x: Int, y: Int)

                fun events() {
                    emit FooEvent(x: 1, y: 2)    // emit
                }
            }
        `
		meter := newTestMemoryGauge()

		inter, err := parseCheckAndInterpretWithOptionsAndMemoryMetering(
			t,
			script,
			ParseCheckAndInterpretOptions{
				Options: []interpreter.Option{
					interpreter.WithContractValueHandler(func(
						inter *interpreter.Interpreter,
						compositeType *sema.CompositeType,
						constructorGenerator func(common.Address) *interpreter.HostFunctionValue,
						invocationRange ast.Range,
					) *interpreter.CompositeValue {
						// Just return a dummy value
						return &interpreter.CompositeValue{}
					}),
				},
			},
			meter,
		)
		require.NoError(t, err)

		_, err = inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindAssignmentStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindBreakStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindContinueStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindIfStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindForStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindWhileStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindReturnStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindSwapStatement))
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindExpressionStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindSwitchStatement))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindEmitStatement))
	})

	t.Run("expressions", func(t *testing.T) {
		t.Parallel()

		script := `
            pub fun main() {
                var a = 5                                // integer expr
                var b = 1.2 + 2.3                        // binary, fixed-point expr
                var c = !true                            // unary, boolean expr
                var d: String? = "hello"                 // string expr
                var e = nil                              // nil expr
                var f: [AnyStruct] = [[], [], []]        // array expr
                var g: {Int: {Int: AnyStruct}} = {1:{}}  // nil expr
                var h <- create bar()                    // create, identifier, invocation
                var i = h.baz                            // member access, identifier x2
                destroy h                                // destroy
                var j = f[0]                             // index access, identifier, integer
                var k = fun() {}                         // function expr
                k()                                      // identifier, invocation
                var l = c ? 1 : 2                        // conditional, identifier, integer x2
                var m = d as AnyStruct                   // casting, identifier
                var n = &d as &AnyStruct                 // reference, casting, identifier
                var o = d!                               // force, identifier
                var p = /public/somepath                 // path
            }

            resource bar {
                let baz: Int
                init() {
                    self.baz = 0x4
                }
            }
        `
		meter := newTestMemoryGauge()

		inter := parseCheckAndInterpretWithMemoryMetering(t, script, meter)

		_, err := inter.Invoke("main")
		require.NoError(t, err)

		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindBooleanExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindNilExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindStringExpression))
		assert.Equal(t, uint64(6), meter.getMemory(common.MemoryKindIntegerExpression))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindFixedPointExpression))
		assert.Equal(t, uint64(7), meter.getMemory(common.MemoryKindArrayExpression))
		assert.Equal(t, uint64(3), meter.getMemory(common.MemoryKindDictionaryExpression))
		assert.Equal(t, uint64(10), meter.getMemory(common.MemoryKindIdentifierExpression))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindInvocationExpression))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindMemberExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindIndexExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindConditionalExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindUnaryExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindBinaryExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindFunctionExpression))
		assert.Equal(t, uint64(2), meter.getMemory(common.MemoryKindCastingExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindCreateExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindDestroyExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindReferenceExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindForceExpression))
		assert.Equal(t, uint64(1), meter.getMemory(common.MemoryKindPathExpression))
	})
}
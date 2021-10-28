// Copyright 2017 PingCAP, Inc.
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

package expression

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/pingcap/tidb/parser/ast"
	"github.com/pingcap/tidb/parser/charset"
	"github.com/pingcap/tidb/parser/mysql"
	"github.com/pingcap/tidb/parser/terror"
	"github.com/pingcap/tidb/sessionctx"
	"github.com/pingcap/tidb/sessionctx/variable"
	"github.com/pingcap/tidb/types"
	"github.com/pingcap/tidb/util/chunk"
	"github.com/pingcap/tidb/util/collate"
	"github.com/pingcap/tidb/util/hack"
	"github.com/stretchr/testify/require"
)

var cryptTests = []struct {
	origin   interface{}
	password interface{}
	crypt    interface{}
}{
	{"", "", ""},
	{"pingcap", "1234567890123456", "2C35B5A4ADF391"},
	{"pingcap", "asdfjasfwefjfjkj", "351CC412605905"},
	{"pingcap123", "123456789012345678901234", "7698723DC6DFE7724221"},
	{"pingcap#%$%^", "*^%YTu1234567", "8634B9C55FF55E5B6328F449"},
	{"pingcap", "", "4A77B524BD2C5C"},
	{"分布式データベース", "pass1234@#$%%^^&", "80CADC8D328B3026D04FB285F36FED04BBCA0CC685BF78B1E687CE"},
	{"分布式データベース", "分布式7782734adgwy1242", "0E24CFEF272EE32B6E0BFBDB89F29FB43B4B30DAA95C3F914444BC"},
	{"pingcap", "密匙", "CE5C02A5010010"},
	{"pingcap数据库", "数据库passwd12345667", "36D5F90D3834E30E396BE3226E3B4ED3"},
	{"数据库5667", 123.435, "B22196D0569386237AE12F8AAB"},
	{nil, "数据库passwd12345667", nil},
}

func TestSQLDecode(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)
	fc := funcs[ast.Decode]
	for _, tt := range cryptTests {
		str := types.NewDatum(tt.origin)
		password := types.NewDatum(tt.password)

		f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{str, password}))
		require.NoError(t, err)
		crypt, err := evalBuiltinFunc(f, chunk.Row{})
		require.NoError(t, err)
		require.Equal(t, types.NewDatum(tt.crypt), toHex(crypt))
	}
	testNullInput(t, ctx, ast.Decode)
}

func TestSQLEncode(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)

	fc := funcs[ast.Encode]
	for _, test := range cryptTests {
		password := types.NewDatum(test.password)
		cryptStr := fromHex(test.crypt)

		f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{cryptStr, password}))
		require.NoError(t, err)
		str, err := evalBuiltinFunc(f, chunk.Row{})

		require.NoError(t, err)
		require.Equal(t, types.NewDatum(test.origin), str)
	}
	testNullInput(t, ctx, ast.Encode)
}

var aesTests = []struct {
	mode   string
	origin interface{}
	params []interface{}
	crypt  interface{}
}{
	// test for ecb
	{"aes-128-ecb", "pingcap", []interface{}{"1234567890123456"}, "697BFE9B3F8C2F289DD82C88C7BC95C4"},
	{"aes-128-ecb", "pingcap123", []interface{}{"1234567890123456"}, "CEC348F4EF5F84D3AA6C4FA184C65766"},
	{"aes-128-ecb", "pingcap", []interface{}{"123456789012345678901234"}, "6F1589686860C8E8C7A40A78B25FF2C0"},
	{"aes-128-ecb", "pingcap", []interface{}{"123"}, "996E0CA8688D7AD20819B90B273E01C6"},
	{"aes-128-ecb", "pingcap", []interface{}{123}, "996E0CA8688D7AD20819B90B273E01C6"},
	{"aes-128-ecb", nil, []interface{}{123}, nil},
	{"aes-192-ecb", "pingcap", []interface{}{"1234567890123456"}, "9B139FD002E6496EA2D5C73A2265E661"},
	{"aes-256-ecb", "pingcap", []interface{}{"1234567890123456"}, "F80DCDEDDBE5663BDB68F74AEDDB8EE3"},
	// test for cbc
	{"aes-128-cbc", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "2ECA0077C5EA5768A0485AA522774792"},
	{"aes-128-cbc", "pingcap", []interface{}{"123456789012345678901234", "1234567890123456"}, "483788634DA8817423BA0934FD2C096E"},
	{"aes-192-cbc", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "516391DB38E908ECA93AAB22870EC787"},
	{"aes-256-cbc", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "5D0E22C1E77523AEF5C3E10B65653C8F"},
	{"aes-256-cbc", "pingcap", []interface{}{"12345678901234561234567890123456", "1234567890123456"}, "A26BA27CA4BE9D361D545AA84A17002D"},
	{"aes-256-cbc", "pingcap", []interface{}{"1234567890123456", "12345678901234561234567890123456"}, "5D0E22C1E77523AEF5C3E10B65653C8F"},
	// test for ofb
	{"aes-128-ofb", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "0515A36BBF3DE0"},
	{"aes-128-ofb", "pingcap", []interface{}{"123456789012345678901234", "1234567890123456"}, "C2A93A93818546"},
	{"aes-192-ofb", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "FE09DCCF14D458"},
	{"aes-256-ofb", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "2E70FCAC0C0834"},
	{"aes-256-ofb", "pingcap", []interface{}{"12345678901234561234567890123456", "1234567890123456"}, "83E2B30A71F011"},
	{"aes-256-ofb", "pingcap", []interface{}{"1234567890123456", "12345678901234561234567890123456"}, "2E70FCAC0C0834"},
	// test for cfb
	{"aes-128-cfb", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "0515A36BBF3DE0"},
	{"aes-128-cfb", "pingcap", []interface{}{"123456789012345678901234", "1234567890123456"}, "C2A93A93818546"},
	{"aes-192-cfb", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "FE09DCCF14D458"},
	{"aes-256-cfb", "pingcap", []interface{}{"1234567890123456", "1234567890123456"}, "2E70FCAC0C0834"},
	{"aes-256-cfb", "pingcap", []interface{}{"12345678901234561234567890123456", "1234567890123456"}, "83E2B30A71F011"},
	{"aes-256-cfb", "pingcap", []interface{}{"1234567890123456", "12345678901234561234567890123456"}, "2E70FCAC0C0834"},
}

func TestAESEncrypt(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)

	fc := funcs[ast.AesEncrypt]
	for _, tt := range aesTests {
		err := variable.SetSessionSystemVar(ctx.GetSessionVars(), variable.BlockEncryptionMode, tt.mode)
		require.NoError(t, err)
		args := []types.Datum{types.NewDatum(tt.origin)}
		for _, param := range tt.params {
			args = append(args, types.NewDatum(param))
		}
		f, err := fc.getFunction(ctx, datumsToConstants(args))
		require.NoError(t, err)
		crypt, err := evalBuiltinFunc(f, chunk.Row{})
		require.NoError(t, err)
		require.Equal(t, types.NewDatum(tt.crypt), toHex(crypt))
	}
	err := variable.SetSessionSystemVar(ctx.GetSessionVars(), variable.BlockEncryptionMode, "aes-128-ecb")
	require.NoError(t, err)
	testNullInput(t, ctx, ast.AesEncrypt)
	testAmbiguousInput(t, ctx, ast.AesEncrypt)
}

func TestAESDecrypt(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)

	fc := funcs[ast.AesDecrypt]
	for _, tt := range aesTests {
		err := variable.SetSessionSystemVar(ctx.GetSessionVars(), variable.BlockEncryptionMode, tt.mode)
		require.NoError(t, err)
		args := []types.Datum{fromHex(tt.crypt)}
		for _, param := range tt.params {
			args = append(args, types.NewDatum(param))
		}
		f, err := fc.getFunction(ctx, datumsToConstants(args))
		require.NoError(t, err)
		str, err := evalBuiltinFunc(f, chunk.Row{})
		require.NoError(t, err)
		if tt.origin == nil {
			require.True(t, str.IsNull())
			continue
		}
		require.Equal(t, types.NewCollationStringDatum(tt.origin.(string), charset.CollationBin, collate.DefaultLen), str)
	}
	err := variable.SetSessionSystemVar(ctx.GetSessionVars(), variable.BlockEncryptionMode, "aes-128-ecb")
	require.NoError(t, err)
	testNullInput(t, ctx, ast.AesDecrypt)
	testAmbiguousInput(t, ctx, ast.AesDecrypt)
}

func testNullInput(t *testing.T, ctx sessionctx.Context, fnName string) {
	err := variable.SetSessionSystemVar(ctx.GetSessionVars(), variable.BlockEncryptionMode, "aes-128-ecb")
	require.NoError(t, err)
	fc := funcs[fnName]
	arg := types.NewStringDatum("str")
	var argNull types.Datum
	f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{arg, argNull}))
	require.NoError(t, err)
	crypt, err := evalBuiltinFunc(f, chunk.Row{})
	require.NoError(t, err)
	require.True(t, crypt.IsNull())

	f, err = fc.getFunction(ctx, datumsToConstants([]types.Datum{argNull, arg}))
	require.NoError(t, err)
	crypt, err = evalBuiltinFunc(f, chunk.Row{})
	require.NoError(t, err)
	require.True(t, crypt.IsNull())
}

func testAmbiguousInput(t *testing.T, ctx sessionctx.Context, fnName string) {
	fc := funcs[fnName]
	arg := types.NewStringDatum("str")
	// test for modes that require init_vector
	err := variable.SetSessionSystemVar(ctx.GetSessionVars(), variable.BlockEncryptionMode, "aes-128-cbc")
	require.NoError(t, err)
	_, err = fc.getFunction(ctx, datumsToConstants([]types.Datum{arg, arg}))
	require.Error(t, err)
	f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{arg, arg, types.NewStringDatum("iv < 16 bytes")}))
	require.NoError(t, err)
	_, err = evalBuiltinFunc(f, chunk.Row{})
	require.Error(t, err)

	// test for modes that do not require init_vector
	err = variable.SetSessionSystemVar(ctx.GetSessionVars(), variable.BlockEncryptionMode, "aes-128-ecb")
	require.NoError(t, err)
	f, err = fc.getFunction(ctx, datumsToConstants([]types.Datum{arg, arg, arg}))
	require.NoError(t, err)
	_, err = evalBuiltinFunc(f, chunk.Row{})
	require.NoError(t, err)
	warnings := ctx.GetSessionVars().StmtCtx.GetWarnings()
	require.GreaterOrEqual(t, len(warnings), 1)
}

func toHex(d types.Datum) (h types.Datum) {
	if d.IsNull() {
		return
	}
	x, _ := d.ToString()
	h.SetString(strings.ToUpper(hex.EncodeToString(hack.Slice(x))), mysql.DefaultCollationName)
	return
}

func fromHex(str interface{}) (d types.Datum) {
	if str == nil {
		return
	}
	if s, ok := str.(string); ok {
		h, _ := hex.DecodeString(s)
		d.SetBytes(h)
	}
	return d
}

var sha1Tests = []struct {
	origin interface{}
	crypt  string
}{
	{"test", "a94a8fe5ccb19ba61c4c0873d391e987982fbbd3"},
	{"c4pt0r", "034923dcabf099fc4c8917c0ab91ffcd4c2578a6"},
	{"pingcap", "73bf9ef43a44f42e2ea2894d62f0917af149a006"},
	{"foobar", "8843d7f92416211de9ebb963ff4ce28125932878"},
	{1024, "128351137a9c47206c4507dcf2e6fbeeca3a9079"},
	{123.45, "22f8b438ad7e89300b51d88684f3f0b9fa1d7a32"},
}

func TestSha1Hash(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)

	fc := funcs[ast.SHA]
	for _, tt := range sha1Tests {
		in := types.NewDatum(tt.origin)
		f, _ := fc.getFunction(ctx, datumsToConstants([]types.Datum{in}))
		crypt, err := evalBuiltinFunc(f, chunk.Row{})
		require.NoError(t, err)
		res, err := crypt.ToString()
		require.NoError(t, err)
		require.Equal(t, tt.crypt, res)
	}
	// test NULL input for sha
	var argNull types.Datum
	f, _ := fc.getFunction(ctx, datumsToConstants([]types.Datum{argNull}))
	crypt, err := evalBuiltinFunc(f, chunk.Row{})
	require.NoError(t, err)
	require.True(t, crypt.IsNull())
}

var sha2Tests = []struct {
	origin     interface{}
	hashLength interface{}
	crypt      interface{}
	validCase  bool
}{
	{"pingcap", 0, "2871823be240f8ecd1d72f24c99eaa2e58af18b4b8ba99a4fc2823ba5c43930a", true},
	{"pingcap", 224, "cd036dc9bec69e758401379c522454ea24a6327b48724b449b40c6b7", true},
	{"pingcap", 256, "2871823be240f8ecd1d72f24c99eaa2e58af18b4b8ba99a4fc2823ba5c43930a", true},
	{"pingcap", 384, "c50955b6b0c7b9919740d956849eedcb0f0f90bf8a34e8c1f4e071e3773f53bd6f8f16c04425ff728bed04de1b63db51", true},
	{"pingcap", 512, "ea903c574370774c4844a83b7122105a106e04211673810e1baae7c2ae7aba2cf07465e02f6c413126111ef74a417232683ce7ba210052e63c15fc82204aad80", true},
	{13572468, 0, "1c91ab1c162fd0cae60a5bb9880f3e7d5a133a65b6057a644b26973d9c55dcfe", true},
	{13572468, 224, "8ad67735bbf49576219f364f4640d595357a440358d15bf6815a16e4", true},
	{13572468, 256, "1c91ab1c162fd0cae60a5bb9880f3e7d5a133a65b6057a644b26973d9c55dcfe", true},
	{13572468.123, 384, "3b4ee302435dc1e15251efd9f3982b1ca6fe4ac778d3260b7bbf3bea613849677eda830239420e448e4c6dc7c2649d89", true},
	{13572468.123, 512, "4820aa3f2760836557dc1f2d44a0ba7596333fdb60c8a1909481862f4ab0921c00abb23d57b7e67a970363cc3fcb78b25b6a0d45cdcac0e87aa0c96bc51f7f96", true},
	{nil, 224, nil, false},
	{"pingcap", nil, nil, false},
	{"pingcap", 123, nil, false},
}

func TestSha2Hash(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)

	fc := funcs[ast.SHA2]
	for _, tt := range sha2Tests {
		str := types.NewDatum(tt.origin)
		hashLength := types.NewDatum(tt.hashLength)
		f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{str, hashLength}))
		require.NoError(t, err)
		crypt, err := evalBuiltinFunc(f, chunk.Row{})
		require.NoError(t, err)
		if tt.validCase {
			res, err := crypt.ToString()
			require.NoError(t, err)
			require.Equal(t, tt.crypt, res)
		} else {
			require.True(t, crypt.IsNull())
		}
	}
}

func TestMD5Hash(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)

	cases := []struct {
		args     interface{}
		expected string
		isNil    bool
		getErr   bool
	}{
		{"", "d41d8cd98f00b204e9800998ecf8427e", false, false},
		{"a", "0cc175b9c0f1b6a831c399e269772661", false, false},
		{"ab", "187ef4436122d1cc2f40dc2b92f0eba0", false, false},
		{"abc", "900150983cd24fb0d6963f7d28e17f72", false, false},
		{123, "202cb962ac59075b964b07152d234b70", false, false},
		{"123", "202cb962ac59075b964b07152d234b70", false, false},
		{123.123, "46ddc40585caa8abc07c460b3485781e", false, false},
		{nil, "", true, false},
	}
	for _, c := range cases {
		f, err := newFunctionForTest(ctx, ast.MD5, primitiveValsToConstants(ctx, []interface{}{c.args})...)
		require.NoError(t, err)
		d, err := f.Eval(chunk.Row{})
		if c.getErr {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			if c.isNil {
				require.Equal(t, types.KindNull, d.Kind())
			} else {
				require.Equal(t, c.expected, d.GetString())
			}
		}
	}
	_, err := funcs[ast.MD5].getFunction(ctx, []Expression{NewZero()})
	require.NoError(t, err)

}

func TestRandomBytes(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)

	fc := funcs[ast.RandomBytes]
	f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{types.NewDatum(32)}))
	require.NoError(t, err)
	out, err := evalBuiltinFunc(f, chunk.Row{})
	require.NoError(t, err)
	require.Equal(t, 32, len(out.GetBytes()))

	f, err = fc.getFunction(ctx, datumsToConstants([]types.Datum{types.NewDatum(1025)}))
	require.NoError(t, err)
	_, err = evalBuiltinFunc(f, chunk.Row{})
	require.Error(t, err)
	f, err = fc.getFunction(ctx, datumsToConstants([]types.Datum{types.NewDatum(-32)}))
	require.NoError(t, err)
	_, err = evalBuiltinFunc(f, chunk.Row{})
	require.Error(t, err)
	f, err = fc.getFunction(ctx, datumsToConstants([]types.Datum{types.NewDatum(0)}))
	require.NoError(t, err)
	_, err = evalBuiltinFunc(f, chunk.Row{})
	require.Error(t, err)

	f, err = fc.getFunction(ctx, datumsToConstants([]types.Datum{types.NewDatum(nil)}))
	require.NoError(t, err)
	out, err = evalBuiltinFunc(f, chunk.Row{})
	require.NoError(t, err)
	require.Equal(t, 0, len(out.GetBytes()))
}

func decodeHex(str string) []byte {
	ret, err := hex.DecodeString(str)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestCompress(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)
	tests := []struct {
		in     interface{}
		expect interface{}
	}{
		{"hello world", string(decodeHex("0B000000789CCA48CDC9C95728CF2FCA4901040000FFFF1A0B045D"))},
		{"", ""},
		{nil, nil},
	}

	fc := funcs[ast.Compress]
	for _, test := range tests {
		arg := types.NewDatum(test.in)
		f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{arg}))
		require.NoErrorf(t, err, "%v", test)
		out, err := evalBuiltinFunc(f, chunk.Row{})
		require.NoErrorf(t, err, "%v", test)
		if test.expect == nil {
			require.Truef(t, out.IsNull(), "%v", test)
			continue
		}
		require.Equalf(t, types.NewCollationStringDatum(test.expect.(string), charset.CollationBin, collate.DefaultLen), out, "%v", test)
	}
}

func TestUncompress(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)
	tests := []struct {
		in     interface{}
		expect interface{}
	}{
		{decodeHex("0B000000789CCB48CDC9C95728CF2FCA4901001A0B045D"), "hello world"},         // zlib result from MySQL
		{decodeHex("0B000000789CCA48CDC9C95728CF2FCA4901040000FFFF1A0B045D"), "hello world"}, // zlib result from TiDB
		{decodeHex("02000000789CCB48CDC9C95728CF2FCA4901001A0B045D"), nil},                   // wrong length in the first four bytes
		{decodeHex(""), ""},
		{"1", nil},
		{"1234", nil},
		{"12345", nil},
		{decodeHex("0B"), nil},
		{decodeHex("0B000000"), nil},
		{decodeHex("0B0000001234"), nil},
		{12345, nil},
		{nil, nil},
	}

	fc := funcs[ast.Uncompress]
	for _, test := range tests {
		arg := types.NewDatum(test.in)
		f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{arg}))
		require.NoErrorf(t, err, "%v", test)
		out, err := evalBuiltinFunc(f, chunk.Row{})
		require.NoErrorf(t, err, "%v", test)
		if test.expect == nil {
			require.Truef(t, out.IsNull(), "%v", test)
			continue
		}
		require.Equalf(t, types.NewCollationStringDatum(test.expect.(string), charset.CollationBin, collate.DefaultLen), out, "%v", test)
	}
}

func TestUncompressLength(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)
	tests := []struct {
		in     interface{}
		expect interface{}
	}{
		{decodeHex("0B000000789CCB48CDC9C95728CF2FCA4901001A0B045D"), int64(11)},         // zlib result from MySQL
		{decodeHex("0B000000789CCA48CDC9C95728CF2FCA4901040000FFFF1A0B045D"), int64(11)}, // zlib result from TiDB
		{decodeHex(""), int64(0)},
		{"1", int64(0)},
		{"123", int64(0)},
		{decodeHex("0B"), int64(0)},
		{decodeHex("0B00"), int64(0)},
		{decodeHex("0B000000"), int64(0x0)},
		{decodeHex("0B0000001234"), int64(0x0B)},
		{12345, int64(875770417)},
		{nil, nil},
	}

	fc := funcs[ast.UncompressedLength]
	for _, test := range tests {
		arg := types.NewDatum(test.in)
		f, err := fc.getFunction(ctx, datumsToConstants([]types.Datum{arg}))
		require.NoErrorf(t, err, "%v", test)
		out, err := evalBuiltinFunc(f, chunk.Row{})
		require.NoErrorf(t, err, "%v", test)
		require.Equalf(t, types.NewDatum(test.expect), out, "%v", test)
	}
}

func TestPassword(t *testing.T) {
	t.Parallel()
	ctx := createContext(t)
	cases := []struct {
		args     interface{}
		expected string
		isNil    bool
		getErr   bool
		getWarn  bool
	}{
		{nil, "", false, false, false},
		{"", "", false, false, false},
		{"abc", "*0D3CED9BEC10A777AEC23CCC353A8C08A633045E", false, false, true},
		{123, "*23AE809DDACAF96AF0FD78ED04B6A265E05AA257", false, false, true},
		{1.23, "*A589EEBA8D3F9E1A34A7EE518FAC4566BFAD5BB6", false, false, true},
		{types.NewDecFromFloatForTest(123.123), "*B15B84262DB34BFB2C817A45A55C405DC7C52BB1", false, false, true},
	}

	warnCount := len(ctx.GetSessionVars().StmtCtx.GetWarnings())
	for _, c := range cases {
		f, err := newFunctionForTest(ctx, ast.PasswordFunc, primitiveValsToConstants(ctx, []interface{}{c.args})...)
		require.NoError(t, err)
		d, err := f.Eval(chunk.Row{})
		require.NoError(t, err)
		if c.isNil {
			require.Equal(t, types.KindNull, d.Kind())
		} else {
			require.Equal(t, c.expected, d.GetString())
		}

		warnings := ctx.GetSessionVars().StmtCtx.GetWarnings()
		if c.getWarn {
			require.Equal(t, warnCount+1, len(warnings))

			lastWarn := warnings[len(warnings)-1]
			require.Truef(t, terror.ErrorEqual(errDeprecatedSyntaxNoReplacement, lastWarn.Err), "err %v", lastWarn.Err)

			warnCount = len(warnings)
		} else {
			require.Equal(t, warnCount, len(warnings))
		}
	}

	_, err := funcs[ast.PasswordFunc].getFunction(ctx, []Expression{NewZero()})
	require.NoError(t, err)
}

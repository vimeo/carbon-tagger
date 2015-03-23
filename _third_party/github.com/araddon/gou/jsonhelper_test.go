package gou

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"testing"
)

var (
	jh JsonHelper
)

func init() {
	SetLogger(log.New(os.Stderr, "", log.Ltime|log.Lshortfile), "debug")
	// create test data
	json.Unmarshal([]byte(`{
		"name":"aaron",
		"ints":[1,2,3,4],
		"int":1,
		"intstr":"1",
		"int64":1234567890,
		"MaxSize" : 1048576,
		"strings":["string1"],
		"stringscsv":"string1,string2",
		"nested":{
			"nest":"string2",
			"strings":["string1"],
			"int":2,
			"list":["value"],
			"nest2":{
				"test":"good"
			}
		},
		"nested2":[
			{"sub":2}
		],
		"period.name":"value"
	}`), &jh)
}

func TestJsonRawWriter(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(`"hello"`)
	raw := json.RawMessage(buf.Bytes())
	bya, _ := json.Marshal(&buf)
	Debug(string(bya))
	bya, _ = json.Marshal(&raw)
	Debug(string(bya))

	/*
		bya, err := json.Marshal(buf)
		Assert(string(bya) == `"hello"`, t, "Should be hello but was %s", string(bya))
		Debug(string(buf.Bytes()), err)
		var jrw JsonRawWriter
		jrw.WriteString(`"hello"`)
		Debug(jrw.Raw())
		bya, err = json.Marshal(jrw.Raw())
		Assert(string(bya) == `"hello"`, t, "Should be hello but was %s", string(bya))
		Debug(string(jrw.Bytes()), err)
	*/
}

//  go test -bench=".*"
//  go test -run="(Util)"

func TestJsonPathGet(t *testing.T) {

	Assert(jh.String("name") == "aaron", t, "should get 'aaron' %s", jh.String("name"))
	Assert(jh.Int("int") == 1, t, "get int ")
	Assert(jh.Int("ints[0]") == 1, t, "get int from array %d", jh.Int("ints[0]"))
	Assert(jh.Int("ints[2]") == 3, t, "get int from array %d", jh.Int("ints[0]"))
	Assert(len(jh.Ints("ints")) == 4, t, "get int array %v", jh.Ints("ints"))
	Assert(jh.Int64("int64") == 1234567890, t, "get int")
	Assert(jh.Int("nested.int") == 2, t, "get int")
	Assert(jh.String("nested.nest") == "string2", t, "should get string %s", jh.String("nested.nest"))
	Assert(jh.String("nested.nest2.test") == "good", t, "should get string %s", jh.String("nested.nest2.test"))
	Assert(jh.String("nested.list[0]") == "value", t, "get string from array")
	Assert(jh.Int("nested2[0].sub") == 2, t, "get int from obj in array %d", jh.Int("nested2[0].sub"))

	Assert(jh.Int("MaxSize") == 1048576, t, "get int, test capitalization? ")
	sl := jh.Strings("strings")
	Assert(len(sl) == 1 && sl[0] == "string1", t, "get strings ")
	sl = jh.Strings("stringscsv")
	Assert(len(sl) == 2 && sl[0] == "string1", t, "get strings ")

	i64, ok := jh.Int64Safe("int64")
	Assert(ok, t, "int64safe ok")
	Assert(i64 == 1234567890, t, "int64safe value")

	i, ok := jh.IntSafe("int")
	Assert(ok, t, "intsafe ok")
	Assert(i == 1, t, "intsafe value")

	l := jh.List("nested2")
	Assert(len(l) == 1, t, "get list")

	jhm := jh.Helpers("nested2")
	Assert(len(jhm) == 1, t, "get list of helpers")
	Assert(jhm[0].Int("sub") == 2, t, "Should get list of helpers")

}

func TestJsonCoercion(t *testing.T) {

	Assert(jh.Int("intstr") == 1, t, "get string as int %s", jh.String("intstr"))
	Assert(jh.String("int") == "1", t, "get int as string %s", jh.String("int"))
	Assert(jh.Int("notint") == -1, t, "get non existent int = 0??? ")

}

func TestJsonPathNotation(t *testing.T) {

	// Now lets test xpath type syntax
	Assert(jh.Int("/MaxSize") == 1048576, t, "get int, test capitalization? ")
	Assert(jh.String("/nested/nest") == "string2", t, "should get string %s", jh.String("/nested/nest"))
	Assert(jh.String("/nested/list[0]") == "value", t, "get string from array")
	// note this one has period in name
	Assert(jh.String("/period.name") == "value", t, "test period in name ")

}

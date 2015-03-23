package gou

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// Convert a slice of bytes into an array by ensuring it is wrapped
//  with []
func MakeJsonList(b []byte) []byte {
	if !bytes.HasPrefix(b, []byte{'['}) {
		b = append([]byte{'['}, b...)
		b = append(b, ']')
	}
	return b
}

func JsonString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

type JsonRawWriter struct {
	bytes.Buffer
}

func (m *JsonRawWriter) MarshalJSON() ([]byte, error) {
	return m.Bytes(), nil
}

func (m *JsonRawWriter) Raw() json.RawMessage {
	return json.RawMessage(m.Bytes())
}

// A simple wrapper tohelp json config files be more easily used
// allows usage such as this
//
//		jh := NewJsonHelper([]byte(`{
//			"name":"string",
//			"ints":[1,5,9,11],
//			"int":1,
//			"int64":1234567890,
//			"MaxSize" : 1048576,
//			"strings":["string1"],
//			"nested":{
//				"nest":"string2",
//				"strings":["string1"],
//				"int":2,
//				"list":["value"],
//				"nest2":{
//					"test":"good"
//				}
//			},
//			"nested2":[
//				{"sub":5}
//			]
//		}`)
//
//		i := jh.Int("nested.int")  // 2
//		i2 := jh.Int("ints[1]")    // 5   array position 1 from [1,5,9,11]
//		s := jh.String("nested.nest")  // "string2"
//
type JsonHelper map[string]interface{}

func NewJsonHelper(b []byte) JsonHelper {
	jh := make(JsonHelper)
	json.Unmarshal(b, &jh)
	return jh
}
func NewJsonHelpers(b []byte) []JsonHelper {
	var jhl []JsonHelper
	json.Unmarshal(MakeJsonList(b), &jhl)
	return jhl
}
func (j JsonHelper) Helper(n string) JsonHelper {
	if v, ok := j[n]; ok {
		switch v.(type) {
		case map[string]interface{}:
			cn := JsonHelper{}
			for n, val := range v.(map[string]interface{}) {
				cn[n] = val
			}
			return cn
		case map[string]string:
			cn := JsonHelper{}
			for n, val := range v.(map[string]string) {
				cn[n] = val
			}
			return cn
		case JsonHelper:
			return v.(JsonHelper)
		default:
			rv := reflect.ValueOf(v)
			Debug("no map? ", v, rv.String(), rv.Type())
		}
	}
	return nil
}
func jsonList(v interface{}) []interface{} {
	switch v.(type) {
	case []interface{}:
		return v.([]interface{})
	}
	return nil
}
func jsonEntry(name string, v interface{}) (interface{}, bool) {
	switch v.(type) {
	case map[string]interface{}:
		if root, ok := v.(map[string]interface{})[name]; ok {
			return root, true
		} else {
			return nil, false
		}
	case JsonHelper:
		return v.(JsonHelper).Get(name), true
	case []interface{}:
		return v, true
	default:
		Debug("no type? ", name, " ", v)
		return nil, false
	}
	return nil, false
}
func (j JsonHelper) Get(n string) interface{} {
	var parts []string
	if strings.Contains(n, "/") {
		parts = strings.Split(n, "/")
		if strings.HasPrefix(n, "/") && len(parts) > 0 {
			parts = parts[1:]
		}
	} else {
		parts = strings.Split(n, ".")
	}
	var root interface{}
	var err error
	var ok, isList, listEntry bool
	var ln, st, idx int
	for ict, name := range parts {
		isList = strings.HasSuffix(name, "[]")
		listEntry = strings.HasSuffix(name, "]") && !isList
		ln, idx = len(name), -1
		if isList || listEntry {
			st = strings.Index(name, "[")
			idx, err = strconv.Atoi(name[st+1 : ln-1])
			name = name[:st]
		}
		if ict == 0 {
			root, ok = j[name]
		} else {
			root, ok = jsonEntry(name, root)
		}
		//Debug(isList, listEntry, " ", name, " ", root, " ", ok, err)
		if !ok {
			return nil
		}
		if isList {
			return jsonList(root)
		} else if listEntry && err == nil {
			if lst := jsonList(root); lst != nil && len(lst) > idx {
				root = lst[idx]
			} else {
				return nil
			}
		}

	}
	return root
}

// Get list of Helpers at given name
func (j JsonHelper) Helpers(n string) []JsonHelper {
	v := j.Get(n)
	if v == nil {
		return nil
	}
	switch v.(type) {
	case []map[string]interface{}:
		hl := make([]JsonHelper, 0)
		for _, val := range v.([]map[string]interface{}) {
			hl = append(hl, val)
		}
		return hl
	case []interface{}:
		if l, ok := v.([]interface{}); ok {
			jhl := make([]JsonHelper, 0)
			for _, item := range l {
				//println(item)
				if jh, ok := item.(map[string]interface{}); ok {
					jhl = append(jhl, jh)
				} else {
					Debug(jh)
				}
			}
			return jhl
		}
	}

	return nil
}
func (j JsonHelper) List(n string) []interface{} {
	v := j.Get(n)
	if l, ok := v.([]interface{}); ok {
		return l
	}
	return nil
}
func (j JsonHelper) Int64(n string) int64 {
	i64, ok := j.Int64Safe(n)
	if !ok {
		return -1
	}
	return i64
}
func (j JsonHelper) String(n string) string {
	if v := j.Get(n); v != nil {
		switch v.(type) {
		case string:
			return v.(string)
		case int:
			return strconv.Itoa(v.(int))
		case int32:
			return strconv.FormatInt(int64(v.(int32)), 10)
		case int64:
			return strconv.FormatInt(v.(int64), 10)
		case uint32:
			return strconv.FormatUint(uint64(v.(uint32)), 10)
		case uint64:
			return strconv.FormatUint(v.(uint64), 10)
		case float64:
			//(v.(float64), 10)
			return strconv.FormatInt(int64(v.(float64)), 10)
		default:
			to := reflect.TypeOf(v)
			Debug("not string type? ", n, " ", v, " ", to.String())
		}
	}
	return ""
}
func (j JsonHelper) Strings(n string) []string {
	if v := j.Get(n); v != nil {
		//Debug(n, " ", v)
		switch v.(type) {
		case string:
			return strings.Split(v.(string), ",")
		case []string:
			//Debug("type []string")
			return v.([]string)
		case []interface{}:
			//Debug("Kind = []interface{} n=", n, "  v=", v)
			sva := make([]string, 0)
			for _, av := range v.([]interface{}) {
				switch av.(type) {
				case string:
					sva = append(sva, av.(string))
				default:
					//Debug("Kind ? ", av)
				}
			}
			return sva
		default:
			//Debug("Kind = ?? ", n, v)
		}
	}
	return nil
}
func (j JsonHelper) Ints(n string) []int {
	v := j.Get(n)
	if v == nil {
		return nil
	}
	if sl, isSlice := v.([]interface{}); isSlice {
		iva := make([]int, 0)
		for _, av := range sl {
			avAsInt, ok := valToInt(av)
			if ok {
				iva = append(iva, avAsInt)
			}
		}
		return iva
	}
	return nil
}
func (j JsonHelper) StringSafe(n string) (string, bool) {
	v := j.Get(n)
	if v != nil {
		if s, ok := v.(string); ok {
			return s, ok
		}
	}
	return "", false
}
func (j JsonHelper) Int(n string) int {
	i, ok := j.IntSafe(n)
	if !ok {
		return -1
	}
	return i
}
func (j JsonHelper) Int64Safe(n string) (int64, bool) {
	v := j.Get(n)
	return valToInt64(v)

}
func (j JsonHelper) IntSafe(n string) (int, bool) {
	v := j.Get(n)
	return valToInt(v)
}

// Given any numeric type (float*, int*, uint*, string) return an int. Returns false if it would
// overflow or if the the argument is not numeric.
func valToInt(i interface{}) (int, bool) {
	i64, ok := valToInt64(i)
	if !ok {
		return -1, false
	}
	if i64 > MaxInt || i64 < MinInt {
		return -1, false
	}
	return int(i64), true
}

// Given any simple type (float*, int*, uint*, string) return an int64. Returns false if it would
// overflow or if the the argument is not numeric.
func valToInt64(i interface{}) (int64, bool) {
	switch x := i.(type) {
	case float32:
		return int64(x), true
	case float64:
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		if x > math.MaxInt64 {
			return 0, false
		}
		return int64(x), true
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return int64(x), true
	case int:
		return int64(x), true
	case uint:
		if uint64(x) > math.MaxInt64 {
			return 0, false
		}
		return int64(x), true
	case string:
		if len(x) > 0 {
			if iv, err := strconv.ParseInt(x, 10, 64); err == nil {
				return iv, true
			}
		}
	}
	return 0, false
}
func (j JsonHelper) Uint64(n string) uint64 {
	v := j.Get(n)
	if v != nil {
		switch v.(type) {
		case int:
			return uint64(v.(int))
		case int64:
			return uint64(v.(int64))
		case uint32:
			return uint64(v.(uint32))
		case uint64:
			return uint64(v.(uint64))
		case float32:
			f := float64(v.(float32))
			return uint64(f)
		case float64:
			f := v.(float64)
			return uint64(f)
		default:
			Debug("no type? ", n, " ", v)
		}
	}
	return 0
}

func (j JsonHelper) BoolSafe(n string) (val bool, ok bool) {
	v := j.Get(n)
	if v != nil {
		switch v.(type) {
		case bool:
			return v.(bool), true
		case string:
			if s := v.(string); len(s) > 0 {
				if b, err := strconv.ParseBool(s); err == nil {
					return b, true
				}
			}
		}
	}
	return false, false
}

func (j JsonHelper) Bool(n string) bool {
	val, ok := j.BoolSafe(n)
	if !ok {
		return false
	}

	return val

}

func (j JsonHelper) MapSafe(n string) (map[string]interface{}, bool) {
	v := j.Get(n)
	if v == nil {
		return nil, false
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return m, true
}

// The following consts are from http://code.google.com/p/go-bit/ (Apache licensed). It
// lets us figure out how wide go ints are, and determine their max and min values.

// Note the use of << to create an untyped constant.
const bitsPerWord = 32 << uint(^uint(0)>>63)

// Implementation-specific size of int and uint in bits.
const BitsPerWord = bitsPerWord // either 32 or 64

// Implementation-specific integer limit values.
const (
	MaxInt  = 1<<(BitsPerWord-1) - 1 // either 1<<31 - 1 or 1<<63 - 1
	MinInt  = -MaxInt - 1            // either -1 << 31 or -1 << 63
	MaxUint = 1<<BitsPerWord - 1     // either 1<<32 - 1 or 1<<64 - 1
)

// Copyright (c) 2012 The 'objc' Package Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package objc

/*
#cgo LDFLAGS: -lobjc -framework Foundation
#define __OBJC2__ 1
#include <objc/runtime.h>
#include <objc/message.h>
#include <stdlib.h>

void *GoObjc_GetObjectSuperClassStruct(void *obj) {
	struct objc_super *s = malloc(sizeof(struct objc_super));
	s->receiver = obj;
	s->super_class = class_getSuperclass(object_getClass(obj));
	return s;
}
*/
import "C"
import (
	"github.com/mkrautz/variadic"
	"math"
	"reflect"
	"unsafe"
)

func unpackStruct(val reflect.Value) []uintptr {
	memArgs := []uintptr{}
	for i := 0; i < val.NumField(); i++ {
		v := val.Field(i)
		kind := v.Kind()
		switch kind {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			memArgs = append(memArgs, uintptr(v.Int()))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			memArgs = append(memArgs, uintptr(v.Uint()))
		case reflect.Float32, reflect.Float64:
			memArgs = append(memArgs, uintptr(math.Float64bits(v.Float())))
		case reflect.Ptr:
			memArgs = append(memArgs, val.Pointer())
		case reflect.Struct:
			args := unpackStruct(v)
			memArgs = append(memArgs, args...)
		}
	}
	return memArgs
}

func sendMsg(obj Object, sendFuncName string, selector string, args ...interface{}) Object {
	// Keep ObjC semantics: messages can be sent to nil objects,
	// but the response is nil.
	if obj.Pointer() == 0 {
		return obj
	}

	sel := selectorWithName(selector)
	if sel == nil {
		return nil
	}

	intArgs := []uintptr{}
	floatArgs := []uintptr{}
	memArgs := []uintptr{}

	typeInfo := simpleTypeInfoForMethod(obj, selector)

	for i, arg := range args {
		switch t := arg.(type) {
		case Object:
			intArgs = append(intArgs, t.Pointer())
		case Selector:
			intArgs = append(intArgs, uintptr(selectorWithName(t.Selector())))
		case uintptr:
			intArgs = append(intArgs, t)
		case int:
			intArgs = append(intArgs, uintptr(t))
		case uint:
			intArgs = append(intArgs, uintptr(t))
		case int8:
			intArgs = append(intArgs, uintptr(t))
		case uint8:
			intArgs = append(intArgs, uintptr(t))
		case int16:
			intArgs = append(intArgs, uintptr(t))
		case uint16:
			intArgs = append(intArgs, uintptr(t))
		case int32:
			intArgs = append(intArgs, uintptr(t))
		case uint32:
			intArgs = append(intArgs, uintptr(t))
		case int64:
			intArgs = append(intArgs, uintptr(t))
		case uint64:
			intArgs = append(intArgs, uintptr(t))
		case bool:
			if t {
				intArgs = append(intArgs, uintptr(1))
			} else {
				intArgs = append(intArgs, uintptr(0))
			}
		case float32:
			floatArgs = append(floatArgs, uintptr(math.Float32bits(t)))
		// Float64 is a bit of a special case. Since SendMsg is a variadic
		// Go function, implicit floats will be of type float64, but we can't
		// be sure that the receiver expects that; they might expect a float32
		// instead.
		//
		// To remedy this, we query the selector's type encoding, and check
		// whether it expects a 32-bit or 64-bit float.
		case float64:
			typeEnc := string(typeInfo[i+3])
			switch typeEnc {
			case encFloat:
				floatArgs = append(floatArgs, uintptr(math.Float32bits(float32(t))))
			case encDouble:
				floatArgs = append(floatArgs, uintptr(math.Float64bits(t)))
			default:
				panic("objc: float argument mismatch")
			}
		default:
			val := reflect.ValueOf(args[i])
			switch val.Kind() {
			case reflect.Ptr:
				intArgs = append(intArgs, val.Pointer())
			case reflect.Uintptr:
				intArgs = append(intArgs, uintptr(val.Uint()))
			case reflect.Struct:
				args := unpackStruct(val)
				memArgs = append(memArgs, args...)
			default:
				panic("unhandled kind")
			}
		}
	}

	fc := variadic.NewFunctionCall(sendFuncName)
	if sendFuncName == "objc_msgSend" {
		fc.Words[0] = obj.Pointer()
	} else if sendFuncName == "objc_msgSendSuper" {
		superPtr := C.GoObjc_GetObjectSuperClassStruct(unsafe.Pointer(obj.Pointer()))
		defer C.free(superPtr)
		fc.Words[0] = uintptr(superPtr)
	} else {
		panic("objc: unknown object.sendMsg sendFuncName")
	}
	fc.Words[1] = uintptr(sel)

	if len(memArgs) > 0 {
		fc.Memory = unsafe.Pointer(&memArgs[0])
		fc.NumMemory = int64(len(memArgs))
	}

	if len(intArgs) > 4 {
		panic("too many int args")
	}
	if len(floatArgs) > 8 {
		panic("too many float args")
	}

	for i, v := range intArgs {
		fc.Words[i+2] = v
	}

	fc.NumFloat = int64(len(floatArgs))
	for i, v := range floatArgs {
		fc.Words[6+i] = v
	}

	if len(typeInfo) > 0 {
		retEnc := string(typeInfo[0])
		if retEnc == encFloat {
			return object{ptr: uintptr(math.Float32bits(fc.CallFloat32()))}
		} else if retEnc == encDouble {
			return object{ptr: uintptr(math.Float64bits(fc.CallFloat64()))}
		}
	}

	return object{ptr: fc.Call()}
}

func (obj object) SendMsg(selector string, args ...interface{}) Object {
	return sendMsg(obj, "objc_msgSend", selector, args...)
}

func (obj object) SendSuperMsg(selector string, args ...interface{}) Object {
	return sendMsg(obj, "objc_msgSendSuper", selector, args...)
}

package bridge

import (
	"go.uber.org/zap/zapcore"
	"time"
)

type objectToMapEncoder struct {
	m          map[string]any
	namespaces []string
}

func (e *objectToMapEncoder) AddArray(key string, arr zapcore.ArrayMarshaler) error {
	enc := &simpleArrayEncoder{}
	if err := arr.MarshalLogArray(enc); err != nil {
		return err
	}
	e.m[key] = enc.values
	return nil
}

func (e *objectToMapEncoder) AddObject(key string, obj zapcore.ObjectMarshaler) error {
	sub := &objectToMapEncoder{m: map[string]any{}}
	if err := obj.MarshalLogObject(sub); err != nil {
		return err
	}
	e.m[key] = sub.m
	return nil
}

func (e *objectToMapEncoder) AddBinary(key string, value []byte)          { e.m[key] = string(value) }
func (e *objectToMapEncoder) AddByteString(key string, value []byte)      { e.m[key] = string(value) }
func (e *objectToMapEncoder) AddBool(key string, value bool)              { e.m[key] = value }
func (e *objectToMapEncoder) AddComplex128(key string, value complex128)  { e.m[key] = value }
func (e *objectToMapEncoder) AddComplex64(key string, value complex64)    { e.m[key] = value }
func (e *objectToMapEncoder) AddDuration(key string, value time.Duration) { e.m[key] = value.String() }
func (e *objectToMapEncoder) AddFloat64(key string, value float64)        { e.m[key] = value }
func (e *objectToMapEncoder) AddFloat32(key string, value float32)        { e.m[key] = value }
func (e *objectToMapEncoder) AddInt(key string, value int)                { e.m[key] = value }
func (e *objectToMapEncoder) AddInt64(key string, value int64)            { e.m[key] = value }
func (e *objectToMapEncoder) AddInt32(key string, value int32)            { e.m[key] = value }
func (e *objectToMapEncoder) AddInt16(key string, value int16)            { e.m[key] = value }
func (e *objectToMapEncoder) AddInt8(key string, value int8)              { e.m[key] = value }
func (e *objectToMapEncoder) AddString(key string, value string)          { e.m[key] = value }
func (e *objectToMapEncoder) AddTime(key string, value time.Time) {
	e.m[key] = value.Format(time.RFC3339Nano)
}
func (e *objectToMapEncoder) AddUint(key string, value uint)       { e.m[key] = value }
func (e *objectToMapEncoder) AddUint64(key string, value uint64)   { e.m[key] = value }
func (e *objectToMapEncoder) AddUint32(key string, value uint32)   { e.m[key] = value }
func (e *objectToMapEncoder) AddUint16(key string, value uint16)   { e.m[key] = value }
func (e *objectToMapEncoder) AddUint8(key string, value uint8)     { e.m[key] = value }
func (e *objectToMapEncoder) AddUintptr(key string, value uintptr) { e.m[key] = value }

func (e *objectToMapEncoder) AddReflected(key string, value any) error {
	e.m[key] = value
	return nil
}

func (e *objectToMapEncoder) OpenNamespace(key string) {
	e.namespaces = append(e.namespaces, key)
}

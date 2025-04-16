package bridge

import (
	"go.uber.org/zap/zapcore"
	"time"
)

type simpleArrayEncoder struct {
	values []any
}

func (s *simpleArrayEncoder) AppendBool(v bool)             { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendByteString(v []byte)     { s.values = append(s.values, string(v)) }
func (s *simpleArrayEncoder) AppendComplex128(v complex128) { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendComplex64(v complex64)   { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendDuration(v time.Duration) {
	s.values = append(s.values, v.String())
}
func (s *simpleArrayEncoder) AppendFloat64(v float64) { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendFloat32(v float32) { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendInt(v int)         { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendInt64(v int64)     { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendInt32(v int32)     { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendInt16(v int16)     { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendInt8(v int8)       { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendString(v string)   { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendTime(v time.Time) {
	s.values = append(s.values, v.Format(time.RFC3339Nano))
}
func (s *simpleArrayEncoder) AppendUint(v uint)       { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendUint64(v uint64)   { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendUint32(v uint32)   { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendUint16(v uint16)   { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendUint8(v uint8)     { s.values = append(s.values, v) }
func (s *simpleArrayEncoder) AppendUintptr(v uintptr) { s.values = append(s.values, v) }

func (s *simpleArrayEncoder) AppendReflected(v any) error {
	s.values = append(s.values, v)
	return nil
}

// ✅ AppendArray: recursively handle nested array
func (s *simpleArrayEncoder) AppendArray(marshaler zapcore.ArrayMarshaler) error {
	nested := &simpleArrayEncoder{}
	if err := marshaler.MarshalLogArray(nested); err != nil {
		return err
	}
	s.values = append(s.values, nested.values)
	return nil
}

// ✅ AppendObject: serialize as map
func (s *simpleArrayEncoder) AppendObject(obj zapcore.ObjectMarshaler) error {
	objMap := make(map[string]any)
	enc := &objectToMapEncoder{m: objMap}
	if err := obj.MarshalLogObject(enc); err != nil {
		return err
	}
	s.values = append(s.values, objMap)
	return nil
}

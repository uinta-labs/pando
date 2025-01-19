package comconnect

import (
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This package is copied to the generated code directory and is used to convert between types.

func unwrap[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func ConvertRefuuidUuidToString(id uuid.UUID) (string, error) {
	if id == uuid.Nil {
		return "", nil
	}
	return id.String(), nil
}

func ConvertPtrReftimeTimeToPtrReftimestamppbTimestamp(at *time.Time) (*timestamppb.Timestamp, error) {
	if at == nil {
		return nil, nil
	}
	return timestamppb.New(*at), nil
}

func ConvertInt32ToInt64(i int32) (int64, error) {
	return int64(i), nil
}

func ConvertSliceOfRefuuidUuidToSliceOfstring(ids []uuid.UUID) ([]string, error) {
	var out []string
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out, nil
}

func ConvertPtrint32ToInt64(id *int32) (int64, error) {
	if id == nil {
		return 0, nil
	}
	return int64(*id), nil
}

func ConvertPtrintToInt32(id *int) (int32, error) {
	if id == nil {
		return 0, nil
	}
	return int32(*id), nil
}

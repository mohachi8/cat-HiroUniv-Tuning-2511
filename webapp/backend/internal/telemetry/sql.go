package telemetry

import (
	"database/sql"
)

func WrapSQLDriver(baseDriver string) string {
	// Jaegerの処理を減らすため、otelsqlのラッピングを無効化
	// パフォーマンスを最優先するため、トレーシングを完全に無効化
	return baseDriver
}

var _ = sql.ErrNoRows

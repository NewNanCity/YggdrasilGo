// Package utils 对象池优化
package utils

import (
	"strings"
	"sync"
)

// 对象池定义（只保留实际使用的）
var (
	// 字符串构建器池
	stringBuilderPool = sync.Pool{
		New: func() interface{} {
			return &strings.Builder{}
		},
	}
)

// GetStringBuilder 从池中获取字符串构建器
func GetStringBuilder() *strings.Builder {
	return stringBuilderPool.Get().(*strings.Builder)
}

// PutStringBuilder 将字符串构建器归还到池中
func PutStringBuilder(builder *strings.Builder) {
	if builder != nil {
		builder.Reset() // 重置内容
		stringBuilderPool.Put(builder)
	}
}

// BuildURL 高性能URL构建
func BuildURL(base, path string) string {
	builder := GetStringBuilder()
	defer PutStringBuilder(builder)

	builder.Grow(len(base) + len(path) + 1) // 预分配容量
	builder.WriteString(base)
	if !strings.HasSuffix(base, "/") && !strings.HasPrefix(path, "/") {
		builder.WriteByte('/')
	}
	builder.WriteString(path)

	return builder.String()
}

// JoinStrings 高性能字符串连接
func JoinStrings(separator string, parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	if len(parts) == 1 {
		return parts[0]
	}

	builder := GetStringBuilder()
	defer PutStringBuilder(builder)

	// 预估容量
	totalLen := len(separator) * (len(parts) - 1)
	for _, part := range parts {
		totalLen += len(part)
	}
	builder.Grow(totalLen)

	// 构建字符串
	builder.WriteString(parts[0])
	for _, part := range parts[1:] {
		builder.WriteString(separator)
		builder.WriteString(part)
	}

	return builder.String()
}

// CopyStringSlice 高性能字符串切片复制
func CopyStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}

	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}

// AppendStrings 高性能字符串切片追加
func AppendStrings(dst []string, src ...string) []string {
	if len(src) == 0 {
		return dst
	}

	// 预分配容量避免多次扩容
	if cap(dst)-len(dst) < len(src) {
		newCap := len(dst) + len(src)
		if newCap < cap(dst)*2 {
			newCap = cap(dst) * 2
		}
		newDst := make([]string, len(dst), newCap)
		copy(newDst, dst)
		dst = newDst
	}

	return append(dst, src...)
}

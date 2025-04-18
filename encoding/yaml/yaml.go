// Copyright 2019 The CUE Authors
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

// Package yaml converts YAML encodings to and from CUE. When converting to CUE,
// comments and position information are retained.
package yaml

import (
	"bytes"
	"io"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	cueyaml "cuelang.org/go/internal/encoding/yaml"
	"cuelang.org/go/internal/source"
	"cuelang.org/go/internal/value"
)

// Extract parses the YAML specified by src to a CUE expression. If
// there's more than one document, the documents will be returned as a
// list. The src argument may be a nil, string, []byte, or io.Reader. If
// src is nil, the result of reading the file specified by filename will
// be used.
func Extract(filename string, src interface{}) (*ast.File, error) {
	data, err := source.ReadAll(filename, src)
	if err != nil {
		return nil, err
	}
	a := []ast.Expr{}
	d := cueyaml.NewDecoder(filename, data)
	for {
		expr, err := d.Decode()
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			if expr != nil {
				a = append(a, expr)
			}
			break
		}
		a = append(a, expr)
	}
	f := &ast.File{Filename: filename}
	switch len(a) {
	case 0:
	case 1:
		switch x := a[0].(type) {
		case *ast.StructLit:
			f.Decls = x.Elts
		default:
			f.Decls = []ast.Decl{&ast.EmbedDecl{Expr: x}}
		}
	default:
		f.Decls = []ast.Decl{&ast.EmbedDecl{Expr: &ast.ListLit{Elts: a}}}
	}
	return f, nil
}

// Encode returns the YAML encoding of v.
func Encode(v cue.Value) ([]byte, error) {
	n := v.Syntax(cue.Final())
	b, err := cueyaml.Encode(n)
	return b, err
}

// EncodeStream returns the YAML encoding of iter, where consecutive values
// of iter are separated with a `---`.
func EncodeStream(iter cue.Iterator) ([]byte, error) {
	// TODO: return an io.Reader and allow asynchronous processing.
	buf := &bytes.Buffer{}
	for i := 0; iter.Next(); i++ {
		if i > 0 {
			buf.WriteString("---\n")
		}
		n := iter.Value().Syntax(cue.Final())
		b, err := cueyaml.Encode(n)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}

// Validate validates the YAML and confirms it matches the constraints
// specified by v. For YAML streams, all values must match v.
func Validate(b []byte, v cue.Value) error {
	ctx := value.OpContext(v)
	_, err := cueyaml.Validate(ctx, b, v)
	return err
}

// Code generated by go generate. DO NOT EDIT.

//go:generate rm pkg.go
//go:generate go run ../gen/gen.go

package time

import (
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/pkg/internal"
)

func init() {
	internal.Register("time", pkg)
}

var _ = adt.TopKind // in case the adt package isn't used

var pkg = &internal.Package{
	Native: []*internal.Builtin{{
		Name:  "Nanosecond",
		Const: "1",
	}, {
		Name:  "Microsecond",
		Const: "1000",
	}, {
		Name:  "Millisecond",
		Const: "1000000",
	}, {
		Name:  "Second",
		Const: "1000000000",
	}, {
		Name:  "Minute",
		Const: "60000000000",
	}, {
		Name:  "Hour",
		Const: "3600000000000",
	}, {
		Name: "Duration",
		Params: []internal.Param{
			{Kind: adt.StringKind},
		},
		Result: adt.BoolKind,
		Func: func(c *internal.CallCtxt) {
			s := c.String(0)
			if c.Do() {
				c.Ret, c.Err = Duration(s)
			}
		},
	}, {
		Name: "FormatDuration",
		Params: []internal.Param{
			{Kind: adt.IntKind},
		},
		Result: adt.StringKind,
		Func: func(c *internal.CallCtxt) {
			d := c.Int64(0)
			if c.Do() {
				c.Ret = FormatDuration(d)
			}
		},
	}, {
		Name: "ParseDuration",
		Params: []internal.Param{
			{Kind: adt.StringKind},
		},
		Result: adt.IntKind,
		Func: func(c *internal.CallCtxt) {
			s := c.String(0)
			if c.Do() {
				c.Ret, c.Err = ParseDuration(s)
			}
		},
	}, {
		Name:  "ANSIC",
		Const: "\"Mon Jan _2 15:04:05 2006\"",
	}, {
		Name:  "UnixDate",
		Const: "\"Mon Jan _2 15:04:05 MST 2006\"",
	}, {
		Name:  "RubyDate",
		Const: "\"Mon Jan 02 15:04:05 -0700 2006\"",
	}, {
		Name:  "RFC822",
		Const: "\"02 Jan 06 15:04 MST\"",
	}, {
		Name:  "RFC822Z",
		Const: "\"02 Jan 06 15:04 -0700\"",
	}, {
		Name:  "RFC850",
		Const: "\"Monday, 02-Jan-06 15:04:05 MST\"",
	}, {
		Name:  "RFC1123",
		Const: "\"Mon, 02 Jan 2006 15:04:05 MST\"",
	}, {
		Name:  "RFC1123Z",
		Const: "\"Mon, 02 Jan 2006 15:04:05 -0700\"",
	}, {
		Name:  "RFC3339",
		Const: "\"2006-01-02T15:04:05Z07:00\"",
	}, {
		Name:  "RFC3339Nano",
		Const: "\"2006-01-02T15:04:05.999999999Z07:00\"",
	}, {
		Name:  "RFC3339Date",
		Const: "\"2006-01-02\"",
	}, {
		Name:  "Kitchen",
		Const: "\"3:04PM\"",
	}, {
		Name:  "Kitchen24",
		Const: "\"15:04\"",
	}, {
		Name:  "January",
		Const: "1",
	}, {
		Name:  "February",
		Const: "2",
	}, {
		Name:  "March",
		Const: "3",
	}, {
		Name:  "April",
		Const: "4",
	}, {
		Name:  "May",
		Const: "5",
	}, {
		Name:  "June",
		Const: "6",
	}, {
		Name:  "July",
		Const: "7",
	}, {
		Name:  "August",
		Const: "8",
	}, {
		Name:  "September",
		Const: "9",
	}, {
		Name:  "October",
		Const: "10",
	}, {
		Name:  "November",
		Const: "11",
	}, {
		Name:  "December",
		Const: "12",
	}, {
		Name:  "Sunday",
		Const: "0",
	}, {
		Name:  "Monday",
		Const: "1",
	}, {
		Name:  "Tuesday",
		Const: "2",
	}, {
		Name:  "Wednesday",
		Const: "3",
	}, {
		Name:  "Thursday",
		Const: "4",
	}, {
		Name:  "Friday",
		Const: "5",
	}, {
		Name:  "Saturday",
		Const: "6",
	}, {
		Name: "Time",
		Params: []internal.Param{
			{Kind: adt.StringKind},
		},
		Result: adt.BoolKind,
		Func: func(c *internal.CallCtxt) {
			s := c.String(0)
			if c.Do() {
				c.Ret, c.Err = Time(s)
			}
		},
	}, {
		Name: "Format",
		Params: []internal.Param{
			{Kind: adt.StringKind},
			{Kind: adt.StringKind},
		},
		Result: adt.BoolKind,
		Func: func(c *internal.CallCtxt) {
			value, layout := c.String(0), c.String(1)
			if c.Do() {
				c.Ret, c.Err = Format(value, layout)
			}
		},
	}, {
		Name: "Parse",
		Params: []internal.Param{
			{Kind: adt.StringKind},
			{Kind: adt.StringKind},
		},
		Result: adt.StringKind,
		Func: func(c *internal.CallCtxt) {
			layout, value := c.String(0), c.String(1)
			if c.Do() {
				c.Ret, c.Err = Parse(layout, value)
			}
		},
	}, {
		Name: "Unix",
		Params: []internal.Param{
			{Kind: adt.IntKind},
			{Kind: adt.IntKind},
		},
		Result: adt.StringKind,
		Func: func(c *internal.CallCtxt) {
			sec, nsec := c.Int64(0), c.Int64(1)
			if c.Do() {
				c.Ret = Unix(sec, nsec)
			}
		},
	}},
}

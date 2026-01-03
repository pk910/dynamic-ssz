package tests

//go:generate go run -cover ../../dynssz-gen -package . -with-streaming -types SimpleBool,SimpleUint8,SimpleUint16,SimpleUint32,SimpleUint64,SimpleTypes1:gen_simple1.go,SimpleTypes1_C1:gen_simple1.go,SimpleTypes2:gen_simple2.go,SimpleTypes3:gen_simple3.go,SimpleTypesWithSpecs:gen_withspecs.go,SimpleTypesWithSpecs2:gen_withspecs2.go,ProgressiveTypes:gen_progressive.go,CustomTypes1:gen_custom.go -legacy -output gen_ssz.go

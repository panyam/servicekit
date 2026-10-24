package http

import "fmt"

func ExampleJSONToQueryString() {
	input := map[string]any{"a": 1, "b": 2}
	queryStr := JSONToQueryString(input)
	fmt.Println(queryStr)

	// Output: a=1&b=2
}

func ExampleNormalizeWSURL() {
	fmt.Println(NormalizeWSURL("http://google.com"))
	fmt.Println(NormalizeWSURL("https://github.com"))

	// Output:
	// ws://google.com
	// wss://github.com
}

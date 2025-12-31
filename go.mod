module github.com/panyam/servicekit

go 1.24.0

require (
	github.com/fernet/fernet-go v0.0.0-20211208181803-9f70042a33ee
	github.com/gorilla/mux v1.8.1
	github.com/gorilla/websocket v1.5.0
	github.com/panyam/gocurrent v0.0.9
	github.com/panyam/goutils v0.1.8
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.10
	gorm.io/gorm v1.23.8
)

require (
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.4 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251029180050-ab9386a59fda // indirect
)

replace github.com/panyam/gocurrent v0.0.9 => ../gocurrent

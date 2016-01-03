// Copyright (c) 2015, Apertus Solutions, LLC
// All rights reserved.

package dbd

import (
	"github.com/godbus/dbus"
)

type Client interface {
	Read(path string) (string, error)
	ReadBinary(path string) ([]byte, error)
	Write(path string, value string) error
	Dump(path string) (string, error)
	Inject(path string, value string) error
	List(path string) ([]string, error)
	Rm(path string) error
	Exists(path string) (bool, error)
}

type Dbd struct {
	conn *dbus.Conn
}

func NewClient() (Client, error) {
	conn, err := dbus.SystemBus()

	if err != nil {
		return nil, err
	}
	return &Dbd{
		conn: conn,
	}, nil
}

//    <method name="read">
//      <arg name="path" type="s" direction="in"/>
//      <arg name="value" type="s" direction="out"/>
//    </method>
func (c *Dbd) Read(path string) (string, error) {
	obj := c.conn.Object("com.citrix.xenclient.db", "/")

	var s string
	err := obj.Call("com.citrix.xenclient.db.read", 0, path).Store(&s)

	return s, err
}

//    <method name="read_binary">
//      <arg name="path" type="s" direction="in"/>
//      <arg name="value" type="ay" direction="out"/>
//    </method>
func (c *Dbd) ReadBinary(path string) ([]byte, error) {
	obj := c.conn.Object("com.citrix.xenclient.db", "/")

	var b []byte
	err := obj.Call("com.citrix.xenclient.db.read_binary", 0, path).Store(&b)

	return b, err
}

//    <method name="write">
//      <arg name="path" type="s" direction="in"/>
//      <arg name="value" type="s" direction="in"/>
//    </method>
func (c *Dbd) Write(path string, value string) error {
	obj := c.conn.Object("com.citrix.xenclient.db", "/")

	call := obj.Call("com.citrix.xenclient.db.write", 0, path, value)

	return call.Err
}

//    <method name="dump">
//      <arg name="path" type="s" direction="in"/>
//      <arg name="value" type="s" direction="out"/>
//    </method>
func (c *Dbd) Dump(path string) (string, error) {
	obj := c.conn.Object("com.citrix.xenclient.db", "/")

	var s string
	err := obj.Call("com.citrix.xenclient.db.dump", 0, path).Store(&s)

	return s, err
}

//    <method name="inject">
//      <arg name="path" type="s" direction="in"/>
//      <arg name="value" type="s" direction="in"/>
//    </method>
func (c *Dbd) Inject(path string, value string) error {
	obj := c.conn.Object("com.citrix.xenclient.db", "/")

	call := obj.Call("com.citrix.xenclient.db.inject", 0, path, value)

	return call.Err
}

//    <method name="list">
//      <arg name="path" type="s" direction="in"/>
//      <arg name="value" type="as" direction="out"/>
//    </method>
func (c *Dbd) List(path string) ([]string, error) {
	obj := c.conn.Object("com.citrix.xenclient.db", "/")

	var s []string
	err := obj.Call("com.citrix.xenclient.db.list", 0, path).Store(&s)

	return s, err
}

//    <method name="rm">
//      <arg name="path" type="s" direction="in"/>
//    </method>
func (c *Dbd) Rm(path string) error {
	obj := c.conn.Object("com.citrix.xenclient.db", "/")

	call := obj.Call("com.citrix.xenclient.db.rm", 0, path)

	return call.Err
}

//    <method name="exists">
//      <arg name="path" type="s" direction="in"/>
//      <arg name="ex" type="b" direction="out"/>
//    </method>
func (c *Dbd) Exists(path string) (bool, error) {
	obj := c.conn.Object("com.citrix.xenclient.db", "/")

	var b bool
	err := obj.Call("com.citrix.xenclient.db.read", 0, path).Store(&b)

	return b, err
}

/**
* @File: conn_manager.go
* @Author: Jason Woo
* @Date: 2023/8/17 15:38
**/

package xnet

import (
	"errors"
	"github.com/dyowoo/xnet/xlog"
	"sync"
)

type IConnManager interface {
	Add(IConnection)                                                       // Add connection
	Remove(IConnection)                                                    // Remove connection
	Get(uint64) (IConnection, error)                                       // Get a connection by ConnID
	Len() int                                                              // Get current number of connections
	ClearConn()                                                            // Remove and stop all connections
	GetAllConnID() []uint64                                                // Get all connection IDs
	Range(func(uint64, IConnection, interface{}) error, interface{}) error // Traverse all connections
}

type ConnManager struct {
	connections map[uint64]IConnection
	connLock    sync.RWMutex
}

func newConnManager() *ConnManager {
	return &ConnManager{
		connections: make(map[uint64]IConnection),
	}
}

func (connMgr *ConnManager) Add(conn IConnection) {

	connMgr.connLock.Lock()
	connMgr.connections[conn.GetConnID()] = conn
	connMgr.connLock.Unlock()

	xlog.InfoF("connection add to connManager successfully: conn num = %d", connMgr.Len())
}

func (connMgr *ConnManager) Remove(conn IConnection) {

	connMgr.connLock.Lock()
	delete(connMgr.connections, conn.GetConnID()) //删除连接信息
	connMgr.connLock.Unlock()

	xlog.InfoF("connection remove connID=%d successfully: conn num = %d", conn.GetConnID(), connMgr.Len())
}

func (connMgr *ConnManager) Get(connID uint64) (IConnection, error) {
	connMgr.connLock.RLock()
	defer connMgr.connLock.RUnlock()

	if conn, ok := connMgr.connections[connID]; ok {
		return conn, nil
	}

	return nil, errors.New("connection not found")
}

func (connMgr *ConnManager) Len() int {
	connMgr.connLock.RLock()
	length := len(connMgr.connections)
	connMgr.connLock.RUnlock()

	return length
}

func (connMgr *ConnManager) ClearConn() {
	connMgr.connLock.Lock()

	for connID, conn := range connMgr.connections {
		//停止
		conn.Stop()
		delete(connMgr.connections, connID)
	}
	connMgr.connLock.Unlock()

	xlog.InfoF("clear all connections successfully: conn num = %d", connMgr.Len())
}

func (connMgr *ConnManager) GetAllConnID() []uint64 {
	ids := make([]uint64, 0, len(connMgr.connections))

	connMgr.connLock.RLock()
	defer connMgr.connLock.RUnlock()

	for id := range connMgr.connections {
		ids = append(ids, id)
	}

	return ids
}

func (connMgr *ConnManager) Range(cb func(uint64, IConnection, interface{}) error, args interface{}) (err error) {
	for connID, conn := range connMgr.connections {
		err = cb(connID, conn, args)
	}

	return err
}

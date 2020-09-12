/*
Copyright 2012 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package singleflight provides a duplicate function call suppression
// mechanism.
package singleflight

import "sync"

// call is an in-flight or completed Do call
// 正在进行中，或已经结束的请求
type call struct {
	wg  sync.WaitGroup // 等待多个协程完成,避免重入
	val interface{}    // 请求得到的正常结果
	err error          // 请求得到的异常结果
}

// Group represents a class of work and forms a namespace in which
// units of work can be executed with duplicate suppression.
// Group 是 singleflight 的最主要的数据结构，管理不同 key 的请求(call)，
// 保证在上一次缓存结果没有关系前，本地不会发送更多的请求
type Group struct {
	mu sync.Mutex       // protects m 一个互斥锁，用来保护m
	m  map[string]*call // lazily initialized 一个懒加载的字典，存储需要被访问的key与其对应的单个访问器
}

// Do executes and returns the results of the given function, making
// sure that only one execution is in-flight for a given key at a
// time. If a duplicate comes in, the duplicate caller waits for the
// original to complete and receives the same results.
// Do 方法，接收 2 个参数，第一个参数是 key，第二个参数是一个函数 fn。
// Do 的作用就是，针对相同的 key，无论 Do 被调用多少次，函数 fn 都只会被调用一次，
// 等待 fn 调用结束了，返回返回值或错误
func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock() // 上锁，防止其他协程进来修改m，干扰接下来的工作
	if g.m == nil {
		// 就是所谓的懒加载，
		// 第一次先创建一个创建一个保存key请就任务的字典
		g.m = make(map[string]*call)
	}
	// 获取到key的执行实例
	if c, ok := g.m[key]; ok { //如果存在说明这个key有正在请求的call
		// key的执行实例已经拿到了，先把整个Group的锁解开，
		// 这里没有IO，预计所不会阻塞其他协程操作其他key太久
		g.mu.Unlock()
		c.wg.Wait()         // 如果之前已经有人发起了这个缓存的请求正在进行中，则等待
		return c.val, c.err // 等待完毕就返回别人请求到的缓存结果
	}
	// 代码走到这里，说明目前当前没有其他协程，在请求这个缓存
	c := new(call) // 发起一个请求
	c.wg.Add(1)    // 准备开始开始工作，这个Group中的其他协程将等待我的请求结果
	g.m[key] = c   // 注册一下这个key的请求任务
	g.mu.Unlock()  // m 缓存的请求注册中心，操作完毕，交出锁

	c.val, c.err = fn() // 执行key的远端请求任务，io部分
	c.wg.Done()         // 请求完毕，通知其他协程，可以那我的结果了

	g.mu.Lock()      // 给注册中心上个锁，准备删除掉本次请求
	delete(g.m, key) //删删删
	g.mu.Unlock()    // m 缓存的请求注册中心，操作完毕，交出锁

	return c.val, c.err // 返回结果
}

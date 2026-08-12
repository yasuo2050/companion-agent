// Package profileextraction 预留只读画像抽取实验边界。
//
// P0 下单主流程不依赖本模块；后续 P1 只允许读取 MongoDB 历史并输出带证据的结构化画像，
// 禁止写回业务库或改变在线下单决策。
package profileextraction

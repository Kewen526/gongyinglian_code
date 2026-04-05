package service

import "errors"

var (
	ErrNoShopPermission = errors.New("无该店铺的查看权限")
)

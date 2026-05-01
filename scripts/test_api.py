#!/usr/bin/env python3
"""
API 连通性测试脚本
测试 https://api.fasvio.com 接口是否正常
"""

import hmac
import hashlib
import time
import json
import urllib.request
import urllib.error
import ssl

BASE_URL = "https://api.fasvio.com"
APP_TOKEN_SECRET = "cd8ef979900320f9c538ef6b699644e348fc8b8271670f936411fac1059e3472"
TOKEN_INTERVAL = 300

USERNAME = "admin"
PASSWORD = "admin123"


def gen_app_token():
    bucket = int(time.time()) // TOKEN_INTERVAL
    return hmac.new(APP_TOKEN_SECRET.encode(), str(bucket).encode(), hashlib.sha256).hexdigest()


def request(method, path, body=None, jwt_token=None):
    url = BASE_URL + path
    data = json.dumps(body).encode() if body else None
    headers = {
        "Content-Type": "application/json",
        "X-App-Token": gen_app_token(),
    }
    if jwt_token:
        headers["Authorization"] = f"Bearer {jwt_token}"

    ctx = ssl.create_default_context()
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, context=ctx, timeout=10) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())
    except Exception as e:
        return 0, {"error": str(e)}


def check(name, status, body, expect_code=0):
    code = body.get("code", -999)
    ok = status == 200 and code == expect_code
    mark = "✅" if ok else "❌"
    print(f"{mark} {name}")
    print(f"   HTTP {status}  code={code}  message={body.get('message', '')}")
    if not ok:
        print(f"   完整响应: {body}")
    print()
    return ok


def main():
    print("=" * 50)
    print("  API 连通性测试")
    print(f"  目标: {BASE_URL}")
    print("=" * 50)
    print()

    # 1. 登录
    status, body = request("POST", "/api/v1/login", {"username": USERNAME, "password": PASSWORD})
    ok = check("登录接口 POST /api/v1/login", status, body)
    if not ok:
        print("登录失败，终止测试")
        return

    jwt_token = body.get("data", {}).get("token", "")
    print(f"   Token 获取成功: {jwt_token[:30]}...\n")

    # 2. 获取账号列表
    status, body = request("GET", "/api/v1/accounts?page=1&page_size=5", jwt_token=jwt_token)
    check("账号列表 GET /api/v1/accounts", status, body)

    # 3. 获取商品列表
    status, body = request("GET", "/api/v1/products?page=1&page_size=5", jwt_token=jwt_token)
    check("商品列表 GET /api/v1/products", status, body)

    # 4. 获取订单列表
    status, body = request("GET", "/api/v1/orders?page=1&page_size=5", jwt_token=jwt_token)
    check("订单列表 GET /api/v1/orders", status, body)

    # 5. 获取钱包信息
    status, body = request("GET", "/api/v1/billing/wallet", jwt_token=jwt_token)
    check("钱包信息 GET /api/v1/billing/wallet", status, body)

    print("=" * 50)
    print("  测试完成")
    print("=" * 50)


if __name__ == "__main__":
    main()

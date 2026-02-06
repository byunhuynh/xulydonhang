import requests
import json

def act_test_connect(api_url, app_id, access_code, org_company_code, timeout=15):
    url = f"{api_url}/api/oauth/actopen/connect"

    payload = {
        "app_id": app_id,
        "access_code": access_code,
        "org_company_code": org_company_code
    }

    try:
        resp = requests.post(url, json=payload, timeout=timeout)
        resp.raise_for_status()

        result = resp.json()
        print("🌐 API Response:", result)

        if not result.get("Success"):
            return {
                "ok": False,
                "message": result.get("ErrorMessage"),
                "error_code": result.get("ErrorCode")
            }

        # Data trả về là JSON string → cần parse tiếp
        raw_data = result.get("Data")
        parsed_data = json.loads(raw_data)

        return {
            "ok": True,
            "access_token": parsed_data.get("access_token"),
            "tenant_code": parsed_data.get("tenant_code"),
            "app_name": parsed_data.get("app_name"),
            "expired_time": parsed_data.get("expired_time"),
            "expired_time_ticks": parsed_data.get("expired_time_ticks"),
            "raw": parsed_data
        }

    except requests.exceptions.RequestException as e:
        return {
            "ok": False,
            "message": f"Request error: {e}"
        }
    except json.JSONDecodeError:
        return {
            "ok": False,
            "message": "Không parse được JSON trong trường Data"
        }

API_URL = "https://actapp.misa.vn/"

result = act_test_connect(
    api_url = API_URL,
    app_id = "0e0a14cf-9e4b-4af9-875b-c490f34a581b",
    access_code = "ZpYXO6DpqmzuKFP06S49hQFbfNZ2xJ4Ie8HPhKsW3lUK9/N0LVWrqfMV/KqVw91wti8mHm2K10ZKSofPfP/5OxzJG9BksHC2c7o7vObqX1w0bRu+Xx7xbL06MW0NJIBoT+gnt8fTwxvlLvkbhbas3P8OkGDi41ixFMaVeAYZ/7Jz8y1OMBdv3YkIw/oRmiljVH6Lm5lTtSQMIuSJMOvD89X6KhPKYwMa5YF0Mo5e8Yd1pnxZ3WXSBu2NbirkgAqtc1fRWYmFsYyTURHVQMBLBg==",
    org_company_code = "congtydemoketnoiact"
)

print(result)

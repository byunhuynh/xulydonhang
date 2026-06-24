import requests

url = "https://apis.haravan.com/com/webhooks.json"

headers = {
    "Authorization": "Bearer F0F9DF4A938B85BF57D490E9134FBAF3D4EB9B9A8A0B84C6F6ADF06AF89BA5E3",
    "Content-Type": "application/json"
}

data = {
    "webhook": {
        "topic": "orders/create",
        "address": "https://script.google.com/macros/s/AKfycbwiVuK9r0ykmzQSbYZ8DFTW01RP_YwHQJ9y2EbLDosQ6VqudDY_9jvn5EUa5Dbhe3psRg/exec",
        "format": "json"
    }
}

res = requests.post(url, json=data, headers=headers)

print(res.status_code)
print(res.text)
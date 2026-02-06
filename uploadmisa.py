from selenium import webdriver
from selenium.webdriver.chrome.service import Service
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.common.keys import Keys
from selenium.common.exceptions import NoSuchElementException
from selenium.common.exceptions import StaleElementReferenceException
from selenium.webdriver.common.action_chains import ActionChains
import os
import time
import re


chromedriver_path = r"chromedriver.exe"
chrome_options = Options()
chrome_options.add_argument("--window-size=1920,1080")
chrome_options.add_argument("--disable-gpu")
chrome_options.add_argument("--headless=new")  # bật nếu cần chạy ẩn
chrome_options.add_experimental_option("excludeSwitches", ["enable-automation"])
chrome_options.add_experimental_option('useAutomationExtension', False)





def baocaocuoingay():
      # Khởi động WebDriver
    service = Service(chromedriver_path)
    driver = webdriver.Chrome(service=service, options=chrome_options)
    try:
        driver.get("https://actapp.misa.vn/app/sa/saorder")
        # Đợi trang đăng nhập
        WebDriverWait(driver, 10).until(EC.presence_of_element_located((By.XPATH, '//*[@id="box-login-right"]/div/div/div[4]/div/div[1]/div[1]/input')))

        # Nhập tài khoản & mật khẩu
        driver.find_element(By.XPATH, '//*[@id="box-login-right"]/div/div/div[4]/div/div[1]/div[1]/input').send_keys("thanhhd@bluehathanh.com")
        driver.find_element(By.XPATH, '//*[@id="box-login-right"]/div/div/div[4]/div/div[1]/div[2]/input').send_keys("@1994123456Bay")

        # Click đăng nhập
        driver.find_element(By.XPATH, '//*[@id="box-login-right"]/div/div/div[4]/div/div[2]/button').click()
        print("✅ Đăng nhập thành công!")

        # chờ tối đa 30s cho đến khi URL chuyển sang 1 trong 2 đường dẫn:
        WebDriverWait(driver, 15).until(
            lambda d: any(substr in d.current_url for substr in [
                "OverUserLogin", 
                "app/sa/saorder"
            ])
        )
        
        current = driver.current_url
        if "OverUserLogin" in current:
            print("⚠️ Quá số người đăng nhập, không thể vào hệ thống!")
        elif "app/sa/saorder" in current:
            print("✅ Đăng nhập và vào đúng trang Import rồi! URL:", current)
        elif "/app/verify" in current:

            print("⚠️ Đăng nhập bị gián đoạn do thiết bị khác. Đang xử lý xác minh...")
            try:
                # Đợi nút Đăng xuất xuất hiện
                WebDriverWait(driver, 5).until(EC.presence_of_element_located(
                    (By.XPATH, '//*[@id="app"]/div[1]/div/div/div[2]/div/div[2]/div[1]/button')))
                
                # Click vào nút đăng xuất (có thể là button hoặc div bên trong)
                dang_xuat = driver.find_element(By.XPATH, '//*[@id="app"]/div[1]/div/div/div[2]/div/div[2]/div[1]/button')
                dang_xuat.click()
                print("🔁 Đã click Đăng xuất.")

                time.sleep(2)  # chờ load lại
                driver.get("https://actapp.misa.vn/app/sa/saorder")
                print("🔄 Đang chuyển lại vào trang Import...")
                
                # Có thể thêm input chờ hoặc retry đăng nhập nếu cần
                input("⏸ Nhấn Enter để tiếp tục kiểm tra sau verify...")

            except Exception as e:
                print("❌ Không tìm thấy nút Đăng xuất hoặc không thể xử lý:", e)

        else:
            # (không khả thi, nhưng để phòng hờ)

            


            print("❌ URL lạ:", current)





        ####### Lọc theo MT
        try:
            button_element = WebDriverWait(driver, 15).until(
                EC.presence_of_element_located((By.XPATH, '//*[@id="main-content"]/div/div/div/div[1]/div[2]/div[1]/div/div[1]/div/div[2]/div[1]/table/thead/tr/div/th[4]/div[1]/div'))
            )
            driver.execute_script("arguments[0].click();", button_element)
            print("✅ Đã click tiêu đề cột để mở filter")
        except:
            print("⛔ Không tìm thấy Nút lọc.")

        try:
            input_element = WebDriverWait(driver, 15).until(
                EC.presence_of_element_located((By.XPATH, '/html/body/div[7]/div[2]/div/div[3]/div/div/div/span[1]/div/input'))
            )
           
            input_element.send_keys("mt")  # Gửi giá trị nếu tìm thấy
        except:
            print("⛔ Không tìm thấy ô input trong thời gian chờ.")

        try:
            button_element = WebDriverWait(driver, 15).until(
                EC.presence_of_element_located((By.XPATH, '/html/body/div[7]/div[3]/div[1]/button/div'))
            )
            driver.execute_script("arguments[0].click();", button_element)
            print("✅ Đã click Lọc Thời gian")
        except:
            print("⛔ Không tìm thấy ô input trong thời gian chờ.")


        

        

        
        ####### Lọc theo Thời gian
        try:
            button_element = WebDriverWait(driver, 15).until(
                EC.presence_of_element_located((By.XPATH, '//*[@id="main-content"]/div/div/div/div[1]/div[2]/div[1]/div/div[1]/div/div[2]/div[1]/table/thead/tr/div/th[17]/div[1]/div'))
            )
            driver.execute_script("arguments[0].click();", button_element)
            print("✅ Đã click tiêu đề cột để mở filter")
        except:
            print("⛔ Không tìm thấy Nút lọc.")

        try:
            input_element = WebDriverWait(driver, 15).until(
                EC.presence_of_element_located((By.XPATH, '/html/body/div[8]/div[2]/div/div[3]/div/span/div/input[1]'))
            )
            input_element.send_keys("24/04/2025")  # Gửi giá trị nếu tìm thấy
        except:
            print("⛔ Không tìm thấy ô input trong thời gian chờ.")

        try:
            button_element = WebDriverWait(driver, 15).until(
                EC.presence_of_element_located((By.XPATH, '/html/body/div[8]/div[3]/div[1]/button/div'))
            )
            driver.execute_script("arguments[0].click();", button_element)
            print("✅ Đã click Lọc MT")
        except:
            print("⛔ Không tìm thấy Nút lọc.")

            


        ############################# lấy tổng đơn


        try:
            tongsodon = WebDriverWait(driver, 15).until(
                EC.presence_of_element_located((By.XPATH, '//*[@id="main-content"]/div/div/div/div[1]/div[2]/div[1]/div/div[1]/div/div[2]/div[1]/div/div/div[1]/div/b'))
            )
            print("📝 Nội dung lấy được:", tongsodon.text)
            tongsodon = int(tongsodon.text.replace(",", "").strip())
            print("📝 Nội dung lấy được:", tongsodon)
        except:
            print("⛔ Không tìm thấy ô tổng đơn trong thời gian chờ.")
        

        danh_sach_don = []
        for i in range(tongsodon):
           
            try:                                             
                ngaydathang = driver.find_element(By.XPATH, f'//*[@id="main-content"]/div/div/div/div[1]/div[2]/div[1]/div/div[1]/div/div[2]/div[1]/table/tbody/tr[{i+1}]/td[2]/div/div/div[2]/span')
                sodonhang  = driver.find_element(By.XPATH, f'//*[@id="main-content"]/div/div/div/div[1]/div[2]/div[1]/div/div[1]/div/div[2]/div[1]/table/tbody/tr[{i+1}]/td[3]/div/div/div[2]/span')
                makhachhang = driver.find_element(By.XPATH, f'//*[@id="main-content"]/div/div/div/div[1]/div[2]/div[1]/div/div[1]/div/div[2]/div[1]/table/tbody/tr[{i+1}]/td[5]/div/div/div/div[2]/span')

                # Ghi vào danh sách
                danh_sach_don.append({
                    "ngay_dat_hang": ngaydathang.text,
                    "so_don_hang": sodonhang.text,
                    "ma_khach_hang": makhachhang.text
                })

                print(f"✅ Dòng {i} - OK")
            except Exception as e:
                print(f"❌ Dòng {i} bị lỗi: {e}")

        print(danh_sach_don)
        
        

        
    except Exception as e:
        print(f"❌ Lỗi khi đăng nhập: {e}")




def uploadmisa():
    # Khởi động WebDriver
    service = Service(chromedriver_path)
    driver = webdriver.Chrome(service=service, options=chrome_options)
    try:
            driver.get("https://actapp.misa.vn/app/Ultilities/Import/3520")
            # Đợi trang đăng nhập
            WebDriverWait(driver, 10).until(EC.presence_of_element_located((By.XPATH, '//*[@id="box-login-right"]/div/div/div[4]/div/div[1]/div[1]/input')))

            # Nhập tài khoản & mật khẩu
            driver.find_element(By.XPATH, '//*[@id="box-login-right"]/div/div/div[4]/div/div[1]/div[1]/input').send_keys("thanhhd@bluehathanh.com")
            driver.find_element(By.XPATH, '//*[@id="box-login-right"]/div/div/div[4]/div/div[1]/div[2]/input').send_keys("@1994123456Bay")

            # Click đăng nhập
            driver.find_element(By.XPATH, '//*[@id="box-login-right"]/div/div/div[4]/div/div[2]/button').click()
            print("✅ Đăng nhập thành công!")

            # chờ tối đa 30s cho đến khi URL chuyển sang 1 trong 2 đường dẫn:
            WebDriverWait(driver, 30).until(
                lambda d: any(substr in d.current_url for substr in [
                    "OverUserLogin", 
                    "/app/Ultilities/Import/3520"
                ])
            )

            current = driver.current_url
            if "OverUserLogin" in current:
                print("⚠️ Quá số người đăng nhập, không thể vào hệ thống!")
            elif "/app/Ultilities/Import/3520" in current:
                print("✅ Đăng nhập và vào đúng trang Import rồi! URL:", current)
            elif "/app/verify#sid=" in current:
                print("⚠️ Đã được đăng nhập thiết bị khác", current)
                dang_xuat = driver.find_element(By.XPATH, '//*[@id="app"]/div[1]/div/div/div[2]/div/div[2]/div[1]/button/div')
                dang_xuat.click()
                driver.get("https://actapp.misa.vn/app/Ultilities/Import/3520")
                #//*[@id="app"]/div[1]/div/div/div[2]/div/div[2]/div[1]/button/div


            else:
                # (không khả thi, nhưng để phòng hờ)
                print("❌ URL lạ:", current)




            # ... sau khi login và đã vào được hệ thống MISA:
            file_input_xpath = '//*[@id="body-layout"]/span/div/div[2]/div[1]/div[2]/div/span/div/div/input'

            # 1. Chờ element <input type="file"> xuất hiện trên DOM
            file_input = WebDriverWait(driver, 10).until(
                EC.presence_of_element_located((By.XPATH, file_input_xpath))
            )

            # 2. Gửi đường dẫn tuyệt đối tới file bạn muốn upload
        # Tạo absolute path
            relative_path = "dondathang.xlsx"
            absolute_path = os.path.abspath(relative_path)
            # Hoặc gán trực tiếp: absolute_path = r"C:\Users\Admin\Documents\dondathang.xlsx"

            print("Đường dẫn tuyệt đối:", absolute_path)

            # Gửi đường dẫn đó lên input
            file_input.send_keys(absolute_path)
            print(f"✅ Đã chọn file: {absolute_path}")

            # 3. (Tùy thuộc trang web) có thể cần click nút "Upload" hoặc "Import" nữa
            upload_btn = driver.find_element(By.XPATH, '//*[@id="footer-layout"]/div/div/div/div[1]/button/div')
            upload_btn.click()
            upload_btn = driver.find_element(By.XPATH, '//*[@id="footer-layout"]/div/div/div/div[1]/button')
            upload_btn.click()
            print("⏳ Đang upload…")
            time.sleep(5)

            # Xpaths
            alt_xpath     = '//*[@id="body-layout"]/div[3]/div[1]/div/div/div[1]'
            valid_xpath   = '//*[@id="body-layout"]/div[3]/div[1]/div/div[1]'
            invalid_xpath = '//*[@id="body-layout"]/div[3]/div[1]/div/div[2]/div[1]'

            # 1) Chờ cho *ít nhất* một trong 2 trạng thái hiển thị:
            WebDriverWait(driver, 30).until(
                lambda d: d.find_elements(By.XPATH, alt_xpath)
                    or (d.find_elements(By.XPATH, valid_xpath)
                        and d.find_elements(By.XPATH, invalid_xpath))
            )

            # 2) Kiểm xem alt_xpath có không
            alt_els = driver.find_elements(By.XPATH, alt_xpath)
            if alt_els:
                # Nếu có, chỉ cần lấy text của nó
                alt_text = alt_els[0].text.strip()
                print("Cả hợp lệ & không hợp lệ gộp:", alt_text)
                # In bảng lỗi
                table = WebDriverWait(driver, 10).until(
                    lambda d: d.find_element(By.XPATH, '//*[@id="body-layout"]/div[5]/div/table')
                )
                for i, row in enumerate(table.find_elements(By.TAG_NAME, "tr")):
                    ths = [h.text for h in row.find_elements(By.TAG_NAME, "th")]
                    tds = [c.text for c in row.find_elements(By.TAG_NAME, "td")]
                    if ths: print(f"Hàng {i} header:", ths)
                    if tds: print(f"Hàng {i} data:  ", tds)





                # Nếu text chứa 2 con số, bạn có thể parse như:
                # valid_count, invalid_count = map(int, re.findall(r'\d+', alt_text))
            else:
                # Ngược lại bắt về cả 2 dòng riêng biệt
                valid_text   = driver.find_element(By.XPATH, valid_xpath).text.split()[0]
                invalid_text = driver.find_element(By.XPATH, invalid_xpath).text.split()[0]
                valid_count   = int(valid_text)
                invalid_count = int(invalid_text)
                print(f"Số dòng hợp lệ:       {valid_count}")
                print(f"Số dòng không hợp lệ: {invalid_count}")

                # Ví dụ logic upload
                if valid_count > 0 and invalid_count == 0:
                    driver.find_element(
                        By.XPATH,
                        '//*[@id="footer-layout"]/div/div/div/div[1]/button/div'
                    ).click()
                    print("✅ Click Upload")
                else:
                    # In bảng lỗi
                    table = WebDriverWait(driver, 10).until(
                        lambda d: d.find_element(By.XPATH, '//*[@id="body-layout"]/div[5]/div/table')
                    )
                    for i, row in enumerate(table.find_elements(By.TAG_NAME, "tr")):
                        ths = [h.text for h in row.find_elements(By.TAG_NAME, "th")]
                        tds = [c.text for c in row.find_elements(By.TAG_NAME, "td")]
                        if ths: print(f"Hàng {i} header:", ths)
                        if tds: print(f"Hàng {i} data:  ", tds)
                
                


            
    except Exception as e:
            print(f"❌ Lỗi khi đăng nhập: {e}")

if __name__ == '__main__':
    #uploadmisa()
    baocaocuoingay()
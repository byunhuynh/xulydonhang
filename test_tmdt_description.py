import unittest

from xulydonhang import ProcessHandler


class TmdtDescriptionTests(unittest.TestCase):
    def test_tiktok_channel_does_not_include_shopee(self):
        description = ProcessHandler.build_tmdt_description(
            channel="tiktokshop",
            shop="Tẩy lồng máy giặt Blue",
            order_number="585484287148721625",
            entry_date="10/08/2026",
            region="HN",
        )

        self.assertEqual(
            description,
            "TMĐT-TikTok - Tẩy lồng máy giặt Blue - "
            "585484287148721625 - Ngày đổ 10/08/2026 - HN",
        )
        self.assertNotIn("Shopee", description)

    def test_shopee_channel_does_not_include_tiktok(self):
        description = ProcessHandler.build_tmdt_description(
            channel="Shopee",
            shop="Blue Official Store",
            order_number="SPX123",
            entry_date="10/08/2026",
            region="LA",
        )

        self.assertEqual(
            description,
            "TMĐT-Shopee - Blue Official Store - SPX123 "
            "- Ngày đổ 10/08/2026 - LA",
        )
        self.assertNotIn("TikTok", description)


if __name__ == "__main__":
    unittest.main()

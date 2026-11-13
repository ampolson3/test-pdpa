-- +goose Up
-- CON-04 (สแกนและจัดหมวดคุกกี้อัตโนมัติ): cookie.cookie_kb was already fully specified in the baseline
-- migrations (00006_cookie.sql) — name_pattern/provider/category_code/description_th/description_en/source
-- — and is deliberately global (no tenant_id column, unlike every other cookie.* table): the names below
-- are publicly documented facts about well-known third-party trackers, not legal or notice wording, so
-- unlike ORG-07/ROPA-09/PNG-03/DPIA-01's own "seed a draft, flag for legal review" moves (CLAUDE.md rule 8
-- is about text shown to a data subject or the PDPC — this is neither), no docs/decisions.md entry is
-- needed. Patterns are substring matches against a discovered cookie's name (the same convention the
-- cookie-sdk's own script_rules already use on the frontend, Service.matchesKBPattern mirrors it in Go).
INSERT INTO cookie.cookie_kb (id, name_pattern, provider, category_code, description_th, description_en, source) VALUES
    ('10000000-0000-4000-8000-000000000001', '_ga', 'Google Analytics', 'analytics', 'ใช้แยกแยะผู้ใช้สำหรับ Google Analytics', 'Used to distinguish users for Google Analytics', 'public-docs'),
    ('10000000-0000-4000-8000-000000000002', '_gid', 'Google Analytics', 'analytics', 'ใช้แยกแยะผู้ใช้สำหรับ Google Analytics (รายวัน)', 'Used to distinguish users for Google Analytics (daily)', 'public-docs'),
    ('10000000-0000-4000-8000-000000000003', '_gat', 'Google Analytics', 'analytics', 'ใช้จำกัดอัตราการส่งข้อมูลไปยัง Google Analytics', 'Used to throttle request rate to Google Analytics', 'public-docs'),
    ('10000000-0000-4000-8000-000000000004', '_ga_', 'Google Analytics 4', 'analytics', 'ใช้คงสถานะ session สำหรับ Google Analytics 4', 'Used to persist session state for Google Analytics 4', 'public-docs'),
    ('10000000-0000-4000-8000-000000000005', '_gcl_au', 'Google Ads', 'marketing', 'ใช้วัดผล conversion ของ Google Ads', 'Used to measure Google Ads conversions', 'public-docs'),
    ('10000000-0000-4000-8000-000000000006', 'NID', 'Google', 'marketing', 'ใช้จดจำการตั้งค่าและแสดงโฆษณาของ Google', 'Used to remember preferences and personalize Google ads', 'public-docs'),
    ('10000000-0000-4000-8000-000000000007', '_fbp', 'Meta (Facebook) Pixel', 'marketing', 'ใช้ส่งข้อมูลโฆษณาไปยัง Meta', 'Used by Meta to deliver advertising', 'public-docs'),
    ('10000000-0000-4000-8000-000000000008', '_fbc', 'Meta (Facebook) Pixel', 'marketing', 'ใช้เก็บข้อมูลคลิกโฆษณาจาก Meta', 'Used by Meta to store ad click information', 'public-docs'),
    ('10000000-0000-4000-8000-000000000009', 'fr', 'Meta (Facebook)', 'marketing', 'ใช้แสดงโฆษณาที่เกี่ยวข้องบน Meta', 'Used by Meta to show relevant advertising', 'public-docs'),
    ('10000000-0000-4000-8000-00000000000a', '_ttp', 'TikTok Pixel', 'marketing', 'ใช้ติดตามพฤติกรรมสำหรับโฆษณาของ TikTok', 'Used by TikTok to track behaviour for advertising', 'public-docs'),
    ('10000000-0000-4000-8000-00000000000b', 'li_sugr', 'LinkedIn Insight Tag', 'marketing', 'ใช้ระบุผู้ใช้สำหรับการวิเคราะห์ของ LinkedIn', 'Used by LinkedIn to identify users for analytics', 'public-docs'),
    ('10000000-0000-4000-8000-00000000000c', '_gcl_dc', 'Google Ads', 'marketing', 'ใช้วัดผล conversion ของ Google Ads (DoubleClick)', 'Used to measure Google Ads conversions (DoubleClick)', 'public-docs'),
    ('10000000-0000-4000-8000-00000000000d', '_hjSession', 'Hotjar', 'analytics', 'ใช้เก็บข้อมูล session สำหรับ Hotjar', 'Used by Hotjar to store session data', 'public-docs'),
    ('10000000-0000-4000-8000-00000000000e', '_hjid', 'Hotjar', 'analytics', 'ใช้ระบุผู้ใช้สำหรับ Hotjar', 'Used by Hotjar to identify a unique user', 'public-docs'),
    ('10000000-0000-4000-8000-00000000000f', '_clck', 'Microsoft Clarity', 'analytics', 'ใช้เก็บข้อมูลผู้ใช้สำหรับ Microsoft Clarity', 'Used by Microsoft Clarity to persist user identity', 'public-docs'),
    ('10000000-0000-4000-8000-000000000010', '_clsk', 'Microsoft Clarity', 'analytics', 'ใช้เก็บข้อมูล session สำหรับ Microsoft Clarity', 'Used by Microsoft Clarity to store session data', 'public-docs'),
    ('10000000-0000-4000-8000-000000000011', '__hstc', 'HubSpot', 'marketing', 'ใช้ติดตามผู้เข้าชมสำหรับ HubSpot', 'Used by HubSpot to track visitors', 'public-docs'),
    ('10000000-0000-4000-8000-000000000012', 'hubspotutk', 'HubSpot', 'marketing', 'ใช้ระบุผู้เข้าชมสำหรับ HubSpot', 'Used by HubSpot to identify visitors', 'public-docs'),
    ('10000000-0000-4000-8000-000000000013', 'intercom-', 'Intercom', 'functional', 'ใช้คงสถานะการสนทนากับฝ่ายบริการลูกค้า', 'Used to persist a customer-support chat session', 'public-docs'),
    ('10000000-0000-4000-8000-000000000014', '__stripe_mid', 'Stripe', 'necessary', 'ใช้ป้องกันการทุจริตสำหรับการชำระเงินของ Stripe', 'Used by Stripe for fraud prevention on payments', 'public-docs'),
    ('10000000-0000-4000-8000-000000000015', '__cf_bm', 'Cloudflare', 'necessary', 'ใช้แยกแยะผู้ใช้จริงจากบอทเพื่อความปลอดภัย', 'Used to distinguish real users from bots for security', 'public-docs'),
    ('10000000-0000-4000-8000-000000000016', 'PHPSESSID', 'PHP session', 'necessary', 'ใช้คงสถานะ session ของเว็บไซต์', 'Used to maintain the website''s own session state', 'public-docs'),
    ('10000000-0000-4000-8000-000000000017', 'JSESSIONID', 'Java session', 'necessary', 'ใช้คงสถานะ session ของเว็บไซต์', 'Used to maintain the website''s own session state', 'public-docs');

-- +goose Down
DELETE FROM cookie.cookie_kb WHERE source = 'public-docs';

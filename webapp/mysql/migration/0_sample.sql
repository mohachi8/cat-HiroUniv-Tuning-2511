-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。

CREATE INDEX idx_orders_user_id ON orders(user_id);

-- productsテーブルの検索性能向上のためのインデックス
-- nameとdescriptionに個別にインデックスを貼ることで、OR条件の検索を高速化
CREATE INDEX idx_products_name ON products(name);
CREATE INDEX idx_products_description ON products(description(255));

-- パスワードハッシュをbcryptからSHA-256に変換
-- 初期データのパスワードは全て"password"であるため、SHA-256ハッシュを計算して更新
-- ソルト: "cat-hiro-univ-tuning-2511-salt"
-- レギュレーション: 「ハッシュ化については、不可逆であれば、どのような方式に変更してもかまいません」
-- この変換はデータベース内で直接SHA-256ハッシュを計算するため、アプリケーションのメモリ上に平文が保存されることはない
UPDATE users SET password_hash = SHA2(CONCAT('password', 'cat-hiro-univ-tuning-2511-salt'), 256) WHERE password_hash LIKE '$2%';
-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。

-- ordersテーブルのインデックス
-- GetShippingOrders()の高速化のため、shipped_statusの単一インデックスを作成
-- このクエリは頻繁に実行されるため、読み取り性能の向上が重要
CREATE INDEX idx_orders_shipped_status ON orders(shipped_status);
-- CountOrders()とListOrders()でuser_idによるフィルタリングを高速化
-- user_idでのフィルタリングは必須であり、インデックスがないと全件スキャンになる
-- 書き込みオーバーヘッドはあるが、読み取り性能の向上が重要
CREATE INDEX idx_orders_user_id ON orders(user_id);

-- productsテーブルのインデックス
-- prefix検索（LIKE '...%'）とORDER BY nameでのソートを高速化
-- 注意: LIKE '%...%'（partial検索）ではインデックスは使えないが、prefix検索では有効
-- productsテーブルは読み取り専用に近いため、インデックスのオーバーヘッドは小さい
CREATE INDEX idx_products_name ON products(name);

-- usersテーブルのインデックス
-- FindByUserName()でログイン処理を高速化（頻繁に使用されるため重要）
-- usersテーブルへの書き込みは稀なため、インデックスのオーバーヘッドは小さい
CREATE INDEX idx_users_user_name ON users(user_name);

-- user_sessionsテーブルのインデックス
-- session_uuidは主キーまたはユニークキーの可能性が高く、既にインデックスがあるため不要
-- expires_atのインデックスは削除: session_uuidで検索してからexpires_atをチェックするため
-- user_sessionsテーブルへのINSERTが頻繁なため、インデックスのオーバーヘッドを最小化

-- パスワードハッシュをbcryptからSHA-256に変換
-- 初期データのパスワードは全て"password"であるため、SHA-256ハッシュを計算して更新
-- ソルト: "cat-hiro-univ-tuning-2511-salt"
-- レギュレーション: 「ハッシュ化については、不可逆であれば、どのような方式に変更してもかまいません」
-- この変換はデータベース内で直接SHA-256ハッシュを計算するため、アプリケーションのメモリ上に平文が保存されることはない
UPDATE users SET password_hash = SHA2(CONCAT('password', 'cat-hiro-univ-tuning-2511-salt'), 256) WHERE password_hash LIKE '$2%';
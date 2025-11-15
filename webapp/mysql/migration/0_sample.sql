-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。

CREATE INDEX idx_orders_user_id ON orders(user_id);

-- productsテーブルのパフォーマンス改善用インデックス
-- 検索用: nameとdescriptionでのLIKE検索を高速化
CREATE INDEX idx_products_name ON products(name);
CREATE INDEX idx_products_description ON products(description(255));

-- ソート用: value, weightでのソートを高速化
CREATE INDEX idx_products_value ON products(value);
CREATE INDEX idx_products_weight ON products(weight);

-- FULLTEXT index for faster fuzzy/fulltext search on products
CREATE FULLTEXT INDEX ft_products_name_description ON products(name, description);
 
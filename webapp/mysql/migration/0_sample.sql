-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。
CREATE INDEX idx_orders_shipped_status_product_id ON orders(shipped_status, product_id);
CREATE INDEX idx_orders_user_id ON orders(user_id);

-- productsテーブルの検索性能向上のためのインデックス
-- nameとdescriptionに個別にインデックスを貼ることで、OR条件の検索を高速化
CREATE INDEX idx_products_name ON products(name);
CREATE INDEX idx_products_description ON products(description(255));

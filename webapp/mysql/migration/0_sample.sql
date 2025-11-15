-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。

CREATE INDEX idx_orders_user_id ON orders(user_id);

-- productsテーブルのパフォーマンス改善用インデックス
-- ソート用: ORDER BY句で使用されるvalue, weightとproduct_idの複合インデックス
-- これにより、ORDER BY value/weight + product_id ASCのソートが高速化される
CREATE INDEX idx_products_value_product_id ON products(value, product_id);
CREATE INDEX idx_products_weight_product_id ON products(weight, product_id);
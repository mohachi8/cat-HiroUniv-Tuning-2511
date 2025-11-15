-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。

CREATE INDEX idx_orders_user_id ON orders(user_id);

-- productsテーブルのパフォーマンス改善用インデックス
-- ソート用: ORDER BY句で使用されるvalue, weightとproduct_idの複合インデックス
-- これにより、ORDER BY value/weight + product_id ASCのソートが高速化される
CREATE INDEX idx_products_value_product_id ON products(value, product_id);
CREATE INDEX idx_products_weight_product_id ON products(weight, product_id);

-- 検索用: nameとdescriptionの部分一致検索を高速化するFULLTEXT INDEX
-- N-gramパーサー（ngram_token_size=5）を使用して、日本語の部分一致検索を最適化
-- MATCH() AGAINST()を使用することで、LIKE '%pattern%'よりも大幅に高速化される
CREATE FULLTEXT INDEX idx_products_name_ft ON products(name) WITH PARSER ngram;
CREATE FULLTEXT INDEX idx_products_description_ft ON products(description) WITH PARSER ngram;
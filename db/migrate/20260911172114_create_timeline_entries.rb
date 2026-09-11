# Owned by fanoutd (Go). Rails only reads it. Kept in the same schema so one migration
# history describes the whole system; the Go service has its own copy of the DDL in
# fanoutd/schema.sql and asserts it at boot.
class CreateTimelineEntries < ActiveRecord::Migration[8.1]
  def change
    create_table :timeline_entries do |t|
      t.bigint :member_id, null: false     # the follower whose feed this row is in
      t.bigint :activity_id, null: false
      t.bigint :author_id, null: false
      t.datetime :activity_created_at, null: false
    end
    # Exactly-once per (follower, activity): the unique index IS the delivery guarantee.
    add_index :timeline_entries, [:member_id, :activity_id], unique: true
    add_index :timeline_entries, [:member_id, :activity_created_at, :activity_id], name: "idx_timeline_keyset"
    add_index :timeline_entries, [:member_id, :author_id], name: "idx_timeline_unfollow"
  end
end

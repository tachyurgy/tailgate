class CreateActivities < ActiveRecord::Migration[8.1]
  def change
    create_table :activities do |t|
      t.references :member, null: false, foreign_key: true
      t.string :kind, null: false
      t.jsonb :payload, null: false
      t.datetime :fanned_out_at
      t.string :fanout_mode          # "push" (materialised to followers) or "pull" (celebrity, merged at read time)
      t.integer :deliveries_count, null: false, default: 0
      t.timestamps
    end
    add_index :activities, [:member_id, :created_at]
    add_index :activities, :created_at
  end
end

class CreateOutboxEvents < ActiveRecord::Migration[8.1]
  def change
    create_table :outbox_events do |t|
      t.references :activity, null: false, foreign_key: true
      t.string :event_type, null: false
      t.integer :attempts, null: false, default: 0
      t.datetime :published_at
      t.text :last_error
      t.timestamps
    end
    add_index :outbox_events, :published_at
    add_index :outbox_events, [:activity_id, :event_type], unique: true
  end
end

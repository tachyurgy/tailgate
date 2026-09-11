class CreateMembers < ActiveRecord::Migration[8.1]
  def change
    create_table :members do |t|
      t.string :handle, null: false
      t.string :display_name, null: false
      t.text :bio
      t.integer :followers_count, null: false, default: 0
      t.integer :following_count, null: false, default: 0
      t.timestamps
    end
    add_index :members, :handle, unique: true
  end
end

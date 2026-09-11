class CreateFollows < ActiveRecord::Migration[8.1]
  def change
    create_table :follows do |t|
      t.references :follower, null: false, foreign_key: { to_table: :members }
      t.references :followee, null: false, foreign_key: { to_table: :members }
      t.timestamps
    end
    # The follow graph edge is unique by construction; a double-click cannot create two edges.
    add_index :follows, [:follower_id, :followee_id], unique: true
    add_check_constraint :follows, "follower_id <> followee_id", name: "no_self_follow"
  end
end

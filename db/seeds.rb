# Deterministic seed: 60 members, a follow graph with one celebrity, ~400 activities.
# Idempotent: re-running does nothing if members exist.
return if Member.exists?
srand(7)
FIRST = %w[Avery Jordan Riley Casey Morgan Quinn Reese Skyler Taylor Dakota Emerson Finley Harper Kendall Logan Parker Rowan Sawyer Blake Cameron]
LAST  = %w[Nguyen Okafor Patel Rivera Schmidt Tanaka Ibrahim Kowalski Larsen Moreau Novak Oduya Petrov Quiroga Rossi Sato Torres Ueda Varga Walsh]
bios  = ["NFL props all season", "esports picks, mostly LoL", "Flex plays only", "sweating parlays since 2019", "NBA unders enjoyer", "here for the ladders", "MLB K props", "CS2 map picks"]

members = 60.times.map do |i|
  Member.create!(handle: "#{FIRST[i % 20].downcase}#{LAST[(i * 7) % 20].downcase}#{i}".gsub(/[^a-z0-9_]/, "")[0, 24],
                 display_name: "#{FIRST[i % 20]} #{LAST[(i * 7) % 20]}", bio: bios.sample)
end
celeb = Member.create!(handle: "picksqueen", display_name: "The Picks Queen", bio: "Sharp. 100k slips shared. Tail responsibly.")

# Follow graph: everyone follows the celebrity; each member follows 6-12 random others.
members.each { |m| m.follow!(celeb) }
members.each do |m|
  (members - [m]).sample(rand(6..12)).each { |o| m.follow!(o) }
end

leagues = %w[NFL NBA MLB NHL LoL CS2]
now = Time.current
400.times do |i|
  author = i % 9 == 0 ? celeb : members.sample
  kind = %w[pick_placed pick_placed pick_placed entry_won streak milestone].sample
  payload = case kind
            when "pick_placed" then { picks: rand(2..6), mode: %w[Power Flex].sample, league: leagues.sample }
            when "entry_won"   then { picks: rand(2..6), mode: %w[Power Flex].sample, league: leagues.sample, multiplier: [3, 5, 10, 25].sample }
            when "streak"      then { days: rand(3..30) }
            else { text: ["hit a 6-pick Flex", "first NHL win of the season", "joined the LoL Worlds ladder", "500th entry"].sample }
            end
  Activity.create!(member: author, kind: kind, payload: payload, created_at: now - rand(0..(14 * 24 * 60)).minutes)
end
puts "seeded #{Member.count} members, #{Follow.count} follows, #{Activity.count} activities (#{OutboxEvent.pending.count} outbox events pending fan-out)"

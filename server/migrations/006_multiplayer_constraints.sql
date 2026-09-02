-- A logical shared quest has at most one copy per assignee.
CREATE UNIQUE INDEX idx_quests_shared_assignee
    ON quests(shared_quest_id, user_id)
    WHERE shared_quest_id IS NOT NULL;

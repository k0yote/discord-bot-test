create table discord_messages (
    id int primary key auto_increment,
    payload json not null,
    user_id bigint not null
);
create table if not exists recordings.user
(
    id       int auto_increment
        primary key,
    name     varchar(128) not null,
    email    varchar(255) not null,
    password varchar(255) not null
);

create table if not exists recordings.album
(
    id     int auto_increment
        primary key,
    title  varchar(128)  not null,
    artist varchar(255)  not null,
    price  decimal(5, 2) not null
);


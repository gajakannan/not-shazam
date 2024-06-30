package utils

import (
	"context"
	"errors"
	"fmt"
	"song-recognition/models"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// godotenv.Load(".env")

var (
	dbUsername = GetEnv("DB_USER")
	dbPassword = GetEnv("DB_PASS")
	dbName     = GetEnv("DB_NAME")
	dbHost     = GetEnv("DB_HOST")
	dbPort     = GetEnv("DB_PORT")

	dbUri = "mongodb://" + dbUsername + ":" + dbPassword + "@" + dbHost + ":" + dbPort + "/" + dbName
)

// DbClient represents a MongoDB client
type DbClient struct {
	client *mongo.Client
}

// NewDbClient creates a new instance of DbClient
func NewDbClient() (*DbClient, error) {
	if dbUsername == "" || dbPassword == "" {
		dbUri = "mongodb://localhost:27017"
	}

	clientOptions := options.Client().ApplyURI(dbUri)
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		return nil, fmt.Errorf("error connecting to MongoDB: %d", err)
	}
	return &DbClient{client: client}, nil
}

// Close closes the underlying MongoDB client
func (db *DbClient) Close() error {
	if db.client != nil {
		return db.client.Disconnect(context.Background())
	}
	return nil
}

func (db *DbClient) StoreFingerprints(fingerprints map[uint32]models.Couple) error {
	collection := db.client.Database("song-recognition").Collection("fingerprints")

	for address, couple := range fingerprints {
		filter := bson.M{"_id": address}
		update := bson.M{
			"$push": bson.M{
				"couples": bson.M{
					"anchorTimeMs": couple.AnchorTimeMs,
					"songID":       couple.SongID,
				},
			},
		}
		opts := options.Update().SetUpsert(true)

		_, err := collection.UpdateOne(context.Background(), filter, update, opts)
		if err != nil {
			return fmt.Errorf("error upserting document: %s", err)
		}
	}

	return nil
}

func (db *DbClient) GetCouples(addresses []uint32) (map[uint32][]models.Couple, error) {
	collection := db.client.Database("song-recognition").Collection("fingerprints")

	couples := make(map[uint32][]models.Couple)

	for _, address := range addresses {
		// Find the document corresponding to the address
		var result bson.M
		err := collection.FindOne(context.Background(), bson.M{"_id": address}).Decode(&result)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				continue
			}
			return nil, fmt.Errorf("error retrieving document for address %d: %s", address, err)
		}

		// Extract couples from the document and append them to the couples map
		var docCouples []models.Couple
		couplesList, ok := result["couples"].(primitive.A)
		if !ok {
			return nil, fmt.Errorf("couples field in document for address %d is not valid", address)
		}

		for _, item := range couplesList {
			itemMap, ok := item.(primitive.M)
			if !ok {
				return nil, fmt.Errorf("invalid couple format in document for address %d", address)
			}

			couple := models.Couple{
				AnchorTimeMs: uint32(itemMap["anchorTimeMs"].(int64)),
				SongID:       uint32(itemMap["songID"].(int64)),
			}
			docCouples = append(docCouples, couple)
		}
		couples[address] = docCouples
	}

	return couples, nil
}

func (db *DbClient) TotalSongs() (int, error) {
	existingSongsCollection := db.client.Database("song-recognition").Collection("songs")
	total, err := existingSongsCollection.CountDocuments(context.Background(), bson.D{})
	if err != nil {
		return 0, err
	}

	return int(total), nil
}

func (db *DbClient) RegisterSong(songTitle, songArtist, ytID string) (uint32, error) {
	existingSongsCollection := db.client.Database("song-recognition").Collection("songs")

	// Create a compound unique index on ytID and key, if it doesn't already exist
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{"ytID", 1}, {"key", 1}},
		Options: options.Index().SetUnique(true),
	}
	_, err := existingSongsCollection.Indexes().CreateOne(context.Background(), indexModel)
	if err != nil {
		return 0, fmt.Errorf("failed to create unique index: %v", err)
	}

	// Attempt to insert the song with ytID and key
	songID := GenerateUniqueID()
	key := GenerateSongKey(songTitle, songArtist)
	_, err = existingSongsCollection.InsertOne(context.Background(), bson.M{"_id": songID, "key": key, "ytID": ytID})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return 0, fmt.Errorf("song with ytID or key already exists: %v", err)
		} else {
			return 0, fmt.Errorf("failed to register song: %v", err)
		}
	}

	return songID, nil
}

type Song struct {
	Title     string
	Artist    string
	YouTubeID string
}

const FILTER_KEYS = "_id | ytID | key"

func (db *DbClient) GetSong(filterKey string, value interface{}) (s Song, songExists bool, e error) {
	if !strings.Contains(FILTER_KEYS, filterKey) {
		return Song{}, false, errors.New("invalid filter key")
	}

	songsCollection := db.client.Database("song-recognition").Collection("songs")
	var song bson.M

	filter := bson.M{filterKey: value}

	err := songsCollection.FindOne(context.Background(), filter).Decode(&song)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return Song{}, false, nil
		}
		return Song{}, false, fmt.Errorf("failed to retrieve song: %v", err)
	}

	ytID := song["ytID"].(string)
	title := strings.Split(song["key"].(string), "---")[0]
	artist := strings.Split(song["key"].(string), "---")[1]

	songInstance := Song{title, artist, ytID}

	return songInstance, true, nil
}

func (db *DbClient) GetSongByID(songID uint32) (Song, bool, error) {
	return db.GetSong("_id", songID)
}

func (db *DbClient) GetSongByYTID(ytID string) (Song, bool, error) {
	return db.GetSong("ytID", ytID)
}

func (db *DbClient) GetSongByKey(key string) (Song, bool, error) {
	return db.GetSong("key", key)
}

func (db *DbClient) DeleteSongByID(songID uint32) error {
	songsCollection := db.client.Database("song-recognition").Collection("songs")

	filter := bson.M{"_id": songID}

	_, err := songsCollection.DeleteOne(context.Background(), filter)
	if err != nil {
		return fmt.Errorf("failed to delete song: %v", err)
	}

	return nil
}

func (db *DbClient) DeleteCollection(collectionName string) error {
	collection := db.client.Database("song-recognition").Collection(collectionName)
	err := collection.Drop(context.Background())
	if err != nil {
		return fmt.Errorf("error deleting collection: %v", err)
	}
	return nil
}

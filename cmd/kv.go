package cmd

/*
#include <stdio.h>
#include <stdlib.h>
#include "nkv_api.h"
#include "nkv_result.h"

struct minio_nkv_handle {
  uint64_t nkv_handle;
  uint64_t container_hash;
  uint64_t network_path_hash;
};

static int minio_nkv_open(char *config, struct minio_nkv_handle *handle) {
  uint64_t instance_uuid = 0;
  nkv_result result;
  result = nkv_open(config, "minio", "msl-ssg-sk01", 1023, &instance_uuid, &handle->nkv_handle);
  return result;
}

static int minio_nkv_open_path(struct minio_nkv_handle *handle, char *ipaddr) {
  uint32_t index = 0;
  uint32_t cnt_count = NKV_MAX_ENTRIES_PER_CALL;
  nkv_container_info *cntlist = malloc(sizeof(nkv_container_info)*NKV_MAX_ENTRIES_PER_CALL);
  memset(cntlist, 0, sizeof(nkv_container_info) * NKV_MAX_ENTRIES_PER_CALL);

  for (int i = 0; i < NKV_MAX_ENTRIES_PER_CALL; i++) {
    cntlist[i].num_container_transport = NKV_MAX_CONT_TRANSPORT;
    cntlist[i].transport_list = malloc(sizeof(nkv_container_transport)*NKV_MAX_CONT_TRANSPORT);
    memset(cntlist[i].transport_list, 0, sizeof(nkv_container_transport)*NKV_MAX_CONT_TRANSPORT);
  }

  int result = nkv_physical_container_list (handle->nkv_handle, index, cntlist, &cnt_count);
  if (result != 0) {
    printf("NKV getting physical container list failed !!, error = %d\n", result);
    exit(1);
  }

  nkv_io_context io_ctx[16];
  memset(io_ctx, 0, sizeof(nkv_io_context) * 16);
  uint32_t io_ctx_cnt = 0;

  for (uint32_t i = 0; i < cnt_count; i++) {
    io_ctx[io_ctx_cnt].container_hash = cntlist[i].container_hash;

    for (int p = 0; p < cntlist[i].num_container_transport; p++) {
      io_ctx[io_ctx_cnt].is_pass_through = 1;
      io_ctx[io_ctx_cnt].container_hash = cntlist[i].container_hash;
      io_ctx[io_ctx_cnt].network_path_hash = cntlist[i].transport_list[p].network_path_hash;

      if(!strcmp(cntlist[i].transport_list[p].ip_addr, ipaddr)) {
              handle->container_hash = cntlist[i].container_hash;
              handle->network_path_hash = cntlist[i].transport_list[p].network_path_hash;
              return 0;
      }
      io_ctx_cnt++;
    }
  }
  return 1;
}

static int minio_nkv_put(struct minio_nkv_handle *handle, void *key, int keyLen, void *value, int valueLen) {
  nkv_result result;
  nkv_io_context ctx;
  ctx.is_pass_through = 1;
  ctx.container_hash = handle->container_hash;
  ctx.network_path_hash = handle->network_path_hash;
  ctx.ks_id = 0;

  const nkv_key  nkvkey = {key, keyLen};
  nkv_store_option option = {0};
  nkv_value nkvvalue = {value, valueLen, 0};
  result = nkv_store_kvp(handle->nkv_handle, &ctx, &nkvkey, &option, &nkvvalue);
  return result;
}

static int minio_nkv_get(struct minio_nkv_handle *handle, void *key, int keyLen, void *value, int valueLen, int *actual_length) {
  nkv_result result;
  nkv_io_context ctx;
  ctx.is_pass_through = 1;
  ctx.container_hash = handle->container_hash;
  ctx.network_path_hash = handle->network_path_hash;
  ctx.ks_id = 0;

  const nkv_key  nkvkey = {key, keyLen};
  nkv_retrieve_option option = {0};

  nkv_value nkvvalue = {value, valueLen, 0};
  result = nkv_retrieve_kvp(handle->nkv_handle, &ctx, &nkvkey, &option, &nkvvalue);
  *actual_length = nkvvalue.actual_length;
  return result;
}

static int minio_nkv_delete(struct minio_nkv_handle *handle, void *key, int keyLen) {
  nkv_result result;
  nkv_io_context ctx;
  ctx.is_pass_through = 1;
  ctx.container_hash = handle->container_hash;
  ctx.network_path_hash = handle->network_path_hash;
  ctx.ks_id = 0;

  const nkv_key  nkvkey = {key, keyLen};
  result = nkv_delete_kvp(handle->nkv_handle, &ctx, &nkvkey);
  return result;
}

*/
import "C"

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"unsafe"
)

type KV struct {
	handle C.struct_minio_nkv_handle
}

func (k *KV) Put(keyStr string, value []byte) error {
	if len(value) > kvMaxValueSize {
		return errValueTooLong
	}
	key := []byte(keyStr)
	var valuePtr unsafe.Pointer
	if len(value) > 0 {
		valuePtr = unsafe.Pointer(&value[0])
	}
	status := C.minio_nkv_put(&k.handle, unsafe.Pointer(&key[0]), C.int(len(key)), valuePtr, C.int(len(value)))
	if status != 0 {
		return errors.New("error during put")
	}
	return nil
}

func (k *KV) Get(keyStr string) ([]byte, error) {
	key := []byte(keyStr)
	var actualLength C.int
	value := make([]byte, kvMaxValueSize)
	status := C.minio_nkv_get(&k.handle, unsafe.Pointer(&key[0]), C.int(len(key)), unsafe.Pointer(&value[0]), C.int(len(value)), &actualLength)
	if status != 0 {
		return nil, errFileNotFound
	}
	return value[:actualLength], nil
}

func (k *KV) Delete(keyStr string) error {
	key := []byte(keyStr)
	status := C.minio_nkv_delete(&k.handle, unsafe.Pointer(&key[0]), C.int(len(key)))
	if status != 0 {
		return errFileNotFound
	}
	return nil
}

var globalKVHandle *KV

const kvVolumesKey = ".minio.sys/kv-volumes"

type kvVolumes struct {
	Version  string
	VolInfos []VolInfo
}

type KVStorage struct {
	kv        KVInterface
	volumes   *kvVolumes
	path      string
	volumesMu sync.RWMutex
}

func newPosix(path string) (StorageAPI, error) {
	kvPath := path
	path = strings.TrimPrefix(path, "/ip/")

	if os.Getenv("MINIO_NKV_EMULATOR") != "" {
		dataDir := pathJoin("/tmp", path, "data")
		os.MkdirAll(dataDir, 0777)
		return &debugStorage{path, &KVStorage{kv: &KVEmulator{dataDir}, path: kvPath}}, nil
	}

	configPath := os.Getenv("MINIO_NKV_CONFIG")
	if configPath == "" {
		return nil, errDiskNotFound
	}

	if globalKVHandle == nil {
		globalKVHandle = &KV{}
		status := C.minio_nkv_open(C.CString(configPath), &globalKVHandle.handle)
		if status != 0 {
			return nil, errDiskNotFound
		}
	}
	kv := *globalKVHandle
	status := C.minio_nkv_open_path(&kv.handle, C.CString(path))
	if status != 0 {
		fmt.Println("unable to open", path)
		return nil, errors.New("unable to open device")
	}
	p := &KVStorage{kv: &kv, path: kvPath}
	if strings.HasSuffix(path, "export-xl/disk1") {
		return &debugStorage{path, p}, nil
	}
	return p, nil
}

func (k *KVStorage) DataKey(id string) string {
	return path.Join(kvDataDir, id)
}

func (k *KVStorage) String() string {
	return k.path
}

func (k *KVStorage) IsOnline() bool {
	return true
}

func (k *KVStorage) LastError() error {
	return nil
}

func (k *KVStorage) Close() error {
	return nil
}

func (k *KVStorage) DiskInfo() (info DiskInfo, err error) {
	return DiskInfo{
		Total: 3 * 1024 * 1024 * 1024 * 1024,
		Free:  3 * 1024 * 1024 * 1024 * 1024,
	}, nil
}

func (k *KVStorage) loadVolumes() (*kvVolumes, error) {
	volumes := &kvVolumes{}
	b, err := k.kv.Get(kvVolumesKey)
	if err != nil {
		return volumes, nil
	}
	if err = json.Unmarshal(b, volumes); err != nil {
		return nil, err
	}
	return volumes, nil
}

func (k *KVStorage) verifyVolume(volume string) error {
	_, err := k.StatVol(volume)
	return err
}

func (k *KVStorage) MakeVol(volume string) (err error) {
	k.volumesMu.Lock()
	defer k.volumesMu.Unlock()
	volumes, err := k.loadVolumes()
	if err != nil {
		return err
	}

	for _, vol := range volumes.VolInfos {
		if vol.Name == volume {
			return errVolumeExists
		}
	}

	volumes.VolInfos = append(volumes.VolInfos, VolInfo{volume, time.Now()})
	b, err := json.Marshal(volumes)
	if err != nil {
		return err
	}
	err = k.kv.Put(kvVolumesKey, b)
	if err != nil {
		return err
	}
	k.volumes = volumes
	return nil
}

func (k *KVStorage) ListVols() (vols []VolInfo, err error) {
	k.volumesMu.Lock()
	defer k.volumesMu.Unlock()
	if k.volumes == nil {
		k.volumes, err = k.loadVolumes()
		if err != nil {
			return nil, err
		}
	}
	for _, vol := range k.volumes.VolInfos {
		if vol.Name == ".minio.sys/multipart" {
			continue
		}
		if vol.Name == ".minio.sys/tmp" {
			continue
		}
		vols = append(vols, vol)
	}
	return vols, nil
}

func (k *KVStorage) StatVol(volume string) (vol VolInfo, err error) {
	k.volumesMu.Lock()
	defer k.volumesMu.Unlock()
	if k.volumes == nil {
		k.volumes, err = k.loadVolumes()
		if err != nil {
			return vol, err
		}
	}
	for _, vol := range k.volumes.VolInfos {
		if vol.Name == volume {
			return VolInfo{vol.Name, vol.Created}, nil
		}
	}
	return vol, errVolumeNotFound
}

func (k *KVStorage) DeleteVol(volume string) (err error) {
	k.volumesMu.Lock()
	defer k.volumesMu.Unlock()
	volumes, err := k.loadVolumes()
	if err != nil {
		return err
	}
	foundIndex := -1
	for i, vol := range volumes.VolInfos {
		if vol.Name == volume {
			foundIndex = i
			break
		}
	}
	if foundIndex == -1 {
		return errVolumeNotFound
	}
	volumes.VolInfos = append(volumes.VolInfos[:foundIndex], volumes.VolInfos[foundIndex+1:]...)

	b, err := json.Marshal(volumes)
	if err != nil {
		return err
	}
	err = k.kv.Put(kvVolumesKey, b)
	if err != nil {
		return err
	}
	k.volumes = volumes
	return err
}

func (k *KVStorage) ListDir(volume, dirPath string, count int) ([]string, error) {
	nskey := pathJoin(volume, dirPath, "xl.json")
	entry := KVNSEntry{}
	b, err := k.kv.Get(nskey)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &entry); err != nil {
		return nil, err
	}
	b, err = k.kv.Get(k.DataKey(entry.IDs[0]))
	if err != nil {
		return nil, err
	}
	xlMeta, err := xlMetaV1UnmarshalJSON(context.Background(), b)
	if err != nil {
		return nil, err
	}
	listEntries := []string{"xl.json"}
	for _, part := range xlMeta.Parts {
		listEntries = append(listEntries, part.Name)
	}
	return listEntries, err
}

func (k *KVStorage) ReadFile(volume string, path string, offset int64, buf []byte, verifier *BitrotVerifier) (n int64, err error) {
	if err = k.verifyVolume(volume); err != nil {
		return 0, err
	}
	return 0, errFileAccessDenied
}

func (k *KVStorage) AppendFile(volume string, path string, buf []byte) (err error) {
	if err = k.verifyVolume(volume); err != nil {
		return err
	}
	return errFileAccessDenied
}

func (k *KVStorage) CreateFile(volume, filePath string, size int64, reader io.Reader) error {
	if err := k.verifyVolume(volume); err != nil {
		return err
	}
	nskey := pathJoin(volume, filePath)
	entry := KVNSEntry{Size: size, ModTime: time.Now()}
	buf := make([]byte, kvMaxValueSize)
	for {
		if size == 0 {
			break
		}
		if size < int64(len(buf)) {
			buf = buf[:size]
		}
		n, err := io.ReadFull(reader, buf)
		if err != nil {
			return err
		}
		size -= int64(n)
		id := mustGetUUID()
		if err = k.kv.Put(k.DataKey(id), buf); err != nil {
			return err
		}
		entry.IDs = append(entry.IDs, id)
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return k.kv.Put(nskey, b)
}

func (k *KVStorage) ReadFileStream(volume, filePath string, offset, length int64) (io.ReadCloser, error) {
	if err := k.verifyVolume(volume); err != nil {
		return nil, err
	}
	nskey := pathJoin(volume, filePath)
	entry := KVNSEntry{}
	b, err := k.kv.Get(nskey)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &entry); err != nil {
		return nil, err
	}
	if length != entry.Size {
		return nil, errUnexpected
	}
	if offset != 0 {
		return nil, errUnexpected
	}
	r, w := io.Pipe()
	go func() {
		for _, id := range entry.IDs {
			data, err := k.kv.Get(k.DataKey(id))
			if err != nil {
				w.CloseWithError(err)
				return
			}
			w.Write(data)
		}
		w.Close()
	}()
	return ioutil.NopCloser(r), nil
}

func (k *KVStorage) RenameFile(srcVolume, srcPath, dstVolume, dstPath string) error {
	if err := k.verifyVolume(srcVolume); err != nil {
		return err
	}
	if err := k.verifyVolume(dstVolume); err != nil {
		return err
	}
	rename := func(src, dst string) error {
		value, err := k.kv.Get(src)
		if err != nil {
			return err
		}
		err = k.kv.Put(dst, value)
		if err != nil {
			return err
		}
		err = k.kv.Delete(src)
		return err
	}
	if !strings.HasSuffix(srcPath, slashSeparator) && !strings.HasSuffix(dstPath, slashSeparator) {
		return rename(pathJoin(srcVolume, srcPath), pathJoin(dstVolume, dstPath))
	}
	if strings.HasSuffix(srcPath, slashSeparator) && strings.HasSuffix(dstPath, slashSeparator) {
		entries, err := k.ListDir(srcVolume, srcPath, -1)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err = rename(pathJoin(srcVolume, srcPath, entry), pathJoin(dstVolume, dstPath, entry)); err != nil {
				return err
			}
		}
		return nil
	}

	return errUnexpected
}

func (k *KVStorage) StatFile(volume string, path string) (fi FileInfo, err error) {
	if err := k.verifyVolume(volume); err != nil {
		return fi, err
	}
	nskey := pathJoin(volume, path)
	entry := KVNSEntry{}
	b, err := k.kv.Get(nskey)
	if err != nil {
		return
	}
	if err = json.Unmarshal(b, &entry); err != nil {
		return
	}
	return FileInfo{
		Volume:  volume,
		Name:    path,
		ModTime: entry.ModTime,
		Size:    entry.Size,
		Mode:    0,
	}, nil
}

func (k *KVStorage) DeleteFile(volume string, path string) (err error) {
	if err := k.verifyVolume(volume); err != nil {
		return err
	}
	nskey := pathJoin(volume, path)
	entry := KVNSEntry{}
	b, err := k.kv.Get(nskey)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &entry); err != nil {
		return err
	}
	for _, id := range entry.IDs {
		k.kv.Delete(k.DataKey(id))
	}
	return k.kv.Delete(nskey)
}

func (k *KVStorage) WriteAll(volume string, filePath string, buf []byte) (err error) {
	if err = k.verifyVolume(volume); err != nil {
		return err
	}
	if filePath == "format.json.tmp" {
		return k.kv.Put(pathJoin(volume, filePath), buf)
	}
	return k.CreateFile(volume, filePath, int64(len(buf)), bytes.NewBuffer(buf))
}

func (k *KVStorage) ReadAll(volume string, filePath string) (buf []byte, err error) {
	if err = k.verifyVolume(volume); err != nil {
		return nil, err
	}

	if filePath == "format.json" {
		buf, err = k.kv.Get(pathJoin(volume, filePath))
		return buf, err
	}
	fi, err := k.StatFile(volume, filePath)
	if err != nil {
		return nil, err
	}
	r, err := k.ReadFileStream(volume, filePath, 0, fi.Size)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(r)
}
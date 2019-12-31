package s.java.util;

import foundation.icon.ee.utils.IObjects;
import i.IObject;
import i.IObjectDeserializer;
import i.IObjectSerializer;
import s.java.lang.Object;

public class UnmodifiableMapEntry<K extends IObject, V extends IObject>
        extends Object
        implements Map.Entry<K, V> {
    K key;
    V value;

    public UnmodifiableMapEntry(K k, V v) {
        key = k;
        value = v;
    }

    public K avm_getKey() {
        return key;
    }

    public V avm_getValue() {
        return value;
    }

    public V avm_setValue(V value) {
        throw new UnsupportedOperationException();
    }

    public boolean avm_equals(IObject o) {
        if (!(o instanceof Map.Entry<?, ?>)) {
            return false;
        }
        Map.Entry<?, ?> e = (Map.Entry<?, ?>) o;
        return IObjects.equals(key, e.avm_getKey())
                && IObjects.equals(value, e.avm_getValue());
    }

    public int avm_hashCode() {
        return IObjects.hashCode(key) ^ IObjects.hashCode(value);
    }

    public UnmodifiableMapEntry(Void ignore, int readIndex) {
        super(ignore, readIndex);
    }

    public void deserializeSelf(java.lang.Class<?> cls, IObjectDeserializer deserializer) {
        super.deserializeSelf(UnmodifiableMapEntry.class, deserializer);

        this.key = (K) deserializer.readObject();
        this.value = (V) deserializer.readObject();
    }

    public void serializeSelf(java.lang.Class<?> cls, IObjectSerializer serializer) {
        super.serializeSelf(UnmodifiableMapEntry.class, serializer);

        serializer.writeObject(key);
        serializer.writeObject(value);
    }
}

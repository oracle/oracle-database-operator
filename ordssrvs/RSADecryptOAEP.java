import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.nio.file.Path;
import java.security.KeyFactory;
import java.security.PrivateKey;
import java.security.spec.PKCS8EncodedKeySpec;
import javax.crypto.Cipher;
import javax.crypto.spec.OAEPParameterSpec;
import javax.crypto.spec.PSource;
import java.security.NoSuchAlgorithmException;
import java.security.InvalidKeyException;
import java.security.spec.InvalidKeySpecException;
import javax.crypto.NoSuchPaddingException;
import javax.crypto.BadPaddingException;
import javax.crypto.IllegalBlockSizeException;
import java.security.InvalidAlgorithmParameterException;
import java.security.spec.MGF1ParameterSpec;
import java.util.Base64;

public class RSADecryptOAEP {

    public static void main(String[] args) throws Exception {

        if (args.length != 2) {
            return;
        }

        String privateKeyPath = "/opt/oracle/sa/encryptionPrivateKey";
        String privateKeyName = args[0];
        String encodedEncryptedPassword = args[1];

        byte[] encodedEncryptedDataBytes = encodedEncryptedPassword.getBytes();
        byte[] encryptedData = Base64.getMimeDecoder().decode(encodedEncryptedDataBytes);

        Path privateKeyDir = Paths.get(privateKeyPath).toAbsolutePath();
        Path requestedPath = privateKeyDir.resolve(privateKeyName).normalize();
        String enc64 = new String(Files.readAllBytes(requestedPath))
                .replace("-----"+"BEGIN PRIVATE KEY"+"-----", "")
                .replace("-----"+"END PRIVATE KEY"+"-----", "")
                .replaceAll("\\s", "");

        byte[] privateKeyBytes = Base64.getDecoder().decode(enc64);

            KeyFactory keyFactory = KeyFactory.getInstance("RSA");
            PKCS8EncodedKeySpec keySpec = new PKCS8EncodedKeySpec(privateKeyBytes);
            PrivateKey privateKey = keyFactory.generatePrivate(keySpec);

            String oaepPadding = "OAEPWithSHA-256AndMGF1Padding";
            Cipher cipher = Cipher.getInstance("RSA/ECB/" + oaepPadding);

            OAEPParameterSpec oaepParams = new OAEPParameterSpec(
                "SHA-256",
                "MGF1",
                new MGF1ParameterSpec("SHA-256"),
                new PSource.PSpecified(new byte[0])
            );

            cipher.init(Cipher.DECRYPT_MODE, privateKey, oaepParams);

            byte[] decryptedData = cipher.doFinal(encryptedData);

            System.out.printf(new String(decryptedData));

    }
}
